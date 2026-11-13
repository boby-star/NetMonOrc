"""Live, controlled fault experiments; ground truth is never sent to the planner."""
import argparse
import csv
from datetime import datetime, timezone
import io
import json
from pathlib import Path
import random
import statistics
import subprocess
import time
from lab import Lab

TRUTH={'http':'http-failure','dns':'dns-failure','tcp':'tcp-failure','office-path':'office-path-failure','multiple':None}
MODES=['static','fixed','adaptive']

def write_private(path,data):
    path=Path(path);path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    tmp=path.with_name(path.name+'.pending');tmp.write_bytes(data);tmp.chmod(0o600);tmp.replace(path)

def save(path,metadata,rows):
    summary=[]
    for mode in MODES:
        for scenario in TRUTH:
            group=[r for r in rows if r['mode']==mode and r['scenario']==scenario]
            if not group:continue
            times=[r['duration_ms'] for r in group if r.get('duration_ms') is not None]
            summary.append({'mode':mode,'scenario':scenario,'trials':len(group),
                            'correct_diagnoses':sum(r.get('diagnosis_correct',False) for r in group),
                            'correct_states':sum(r.get('state_correct',False) for r in group),
                            'harness_errors':sum(r.get('status')=='harness_error' for r in group),
                            'inconclusive':sum(r.get('status')=='inconclusive' for r in group),
                            'median_duration_ms':statistics.median(times) if times else None,
                            'completed_probes':[r.get('completed_probes') for r in group]})
    write_private(path,json.dumps({'metadata':metadata,'summary':summary,'trials':rows},indent=2).encode())
    buffer=io.StringIO();fields=['repetition','mode','scenario','expected_state','state','state_correct','status','prediction','diagnosis_correct','duration_ms','assessment_sample_after_control_ms','reserved_checks','probe_observations','completed_probes','execution_failures','reason']
    writer=csv.DictWriter(buffer,fieldnames=fields,extrasaction='ignore');writer.writeheader();writer.writerows(rows)
    write_private(Path(path).with_suffix('.csv'),buffer.getvalue().encode())


def trial(lab,mode,scenario,repetition):
    lab.healthy()
    previous=lab.diagnostic()
    previous_id=previous['id'] if previous else None
    control_start=datetime.now(timezone.utc).isoformat()
    lab.inject(scenario)
    control_done=time.monotonic()
    applied_wall=time.time()
    expected_state='degraded' if scenario=='office-path' else 'down'
    # Sampling is gated by fresh evidence, never by the expected answer.
    # A wrong state is recorded as wrong rather than waited away.
    fresh_query=("SELECT count(*) FROM (SELECT DISTINCT agent_id,check_id FROM nmo_observations "
                 "WHERE diagnostic_task_id='' AND agent_id IN ('branch','dc') AND check_id IN ('http','tcp','dns') "
                 "AND finished_at>to_timestamp("+str(applied_wall)+")) fresh")
    lab.wait(lambda:lab.sql(fresh_query)=='6','fresh evidence from every planned source/check',seconds=40)
    evidence_committed=time.time()
    def assessed_after_gate():
        a=lab.assessment()
        if not a:return None
        at=datetime.fromisoformat(a['at'].replace('Z','+00:00')).timestamp()
        return a if at>evidence_committed else None
    a=lab.wait(assessed_after_gate,'assessment after fresh evidence commit',seconds=10)
    sample_ms=round((time.monotonic()-control_done)*1000)
    r=lab.wait(lambda:(r if (r:=lab.diagnostic()) and r['id']!=previous_id and
                      r['result']['status'] in ['completed','inconclusive'] else None),
               'terminal diagnostic report',seconds=100)
    # Runtime IDs are checked before embedding in this fixed experiment query.
    run=r['id']
    if len(run)!=32 or any(c not in '0123456789abcdef' for c in run):
        raise RuntimeError('invalid diagnostic ID in report')
    tasks=json.loads(lab.sql("SELECT COALESCE(json_agg(json_build_object('id',t.id,'agent',t.agent_id,'check',t.check_id,'state',t.state,'observation',o.payload)),'[]'::json) FROM nmo_diagnostic_tasks t LEFT JOIN nmo_observations o ON o.diagnostic_task_id=t.id WHERE t.run_id='"+run+"'"))
    observations=[t['observation'] for t in tasks if t['observation'] is not None]
    result=r['result']
    prediction=result['hypotheses'][0] if result['status']=='completed' and len(result['hypotheses'])==1 else None
    correct=prediction==TRUTH[scenario] if TRUTH[scenario] else result['status']=='inconclusive' and prediction is None
    return {'repetition':repetition,'mode':mode,'scenario':scenario,'ground_truth':TRUTH[scenario],
            'fault_control_started_at':control_start,'expected_state':expected_state,'state':a['state'],
            'state_correct':a['state']==expected_state,'status':result['status'],'prediction':prediction,
            'diagnosis_correct':correct,'duration_ms':result.get('durationMs'),
            'assessment_sample_after_control_ms':sample_ms,'reserved_checks':r['reservedChecks'],
            'probe_observations':len(observations),
            'completed_probes':sum(o['execution']=='completed' for o in observations),
            'execution_failures':sum(o['execution']=='execution_failed' for o in observations),
            'reason':result['reason'],'assessment':a,'diagnostic':r,'tasks':tasks}


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project',default='netmonorc-lab')
    parser.add_argument('--prefix',default='172.30')
    parser.add_argument('--repetitions',type=int,default=1)
    parser.add_argument('--seed',type=int,default=7)
    parser.add_argument('--modes',default=','.join(MODES))
    parser.add_argument('--scenarios',default=','.join(TRUTH))
    parser.add_argument('--output',default='.local/lab-experiment.json')
    args=parser.parse_args()
    modes=args.modes.split(',');scenarios=args.scenarios.split(',')
    if not 1<=args.repetitions<=30 or len(set(modes))!=len(modes) or any(m not in MODES for m in modes) or not modes or len(set(scenarios))!=len(scenarios) or any(s not in TRUTH for s in scenarios):
        parser.error('use 1..30 repetitions and unique known modes/scenarios')
    output=Path(args.output)
    if output.exists() or output.with_suffix('.csv').exists():
        parser.error('output already exists; choose a new path to preserve previous evidence')
    lab=Lab(args.project,args.prefix);lab.require()
    if lab.mode() is None:parser.error('start the prepared stand before running experiments')
    # Validate compiled equivalence, including defaults, through the Go compiler.
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-experiment',
                '-adaptive','/run/netmonorc/adaptive.yaml','-fixed','/run/netmonorc/fixed.yaml','-validate-only')
    head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=Path(__file__).resolve().parents[1]).decode().strip()
    dirty=bool(subprocess.check_output(['git','status','--porcelain'],cwd=Path(__file__).resolve().parents[1]))
    metadata={'source':'live-compose-fault-injection','started_at':datetime.now(timezone.utc).isoformat(),
              'git_head':head,'git_dirty':dirty,'project':args.project,'seed':args.seed,
              'repetitions':args.repetitions,'clock':'shared host kernel; Engine duration uses monotonic clock',
              'traffic_bytes_measured':False,'modes':modes,'scenarios':scenarios,'office_fault_control':'explicit prohibit route in office namespace',
              'state_sampling':'fresh evidence from all planned check/source pairs; expected state is not a polling predicate',
              'limitations':['small pilot is not statistical evidence','shared host resources',
                             'assessment sampled after fresh observations, without waiting for an expected state',
                             'completed_probes excludes unknown physical completion on execution failure']}
    for service in ['controller','target']:
        container=lab.compose('ps','--quiet',service).decode().strip()
        metadata[service+'_image_id']=subprocess.check_output(['docker','inspect','--format','{{.Image}}',container]).decode().strip()
    rows=[];rng=random.Random(args.seed)
    try:
        for repetition in range(1,args.repetitions+1):
            order=modes.copy();rng.shuffle(order)
            for mode in order:
                lab.healthy();lab.up(mode)
                faults=scenarios.copy();rng.shuffle(faults)
                for scenario in faults:
                    print(f'RUN {repetition} {mode} {scenario}',flush=True)
                    try:
                        row=trial(lab,mode,scenario,repetition)
                    except (RuntimeError,subprocess.CalledProcessError) as e:
                        rows.append({'repetition':repetition,'mode':mode,'scenario':scenario,
                                     'status':'harness_error','reason':type(e).__name__})
                        save(output,metadata,rows)
                        raise
                    rows.append(row);save(output,metadata,rows)
                    print(f"RESULT {row['status']} prediction={row['prediction']} correct={row['diagnosis_correct']} completed_probes={row['completed_probes']}",flush=True)
    finally:
        metadata['finished_at']=datetime.now(timezone.utc).isoformat()
        save(output,metadata,rows)
        lab.healthy()
        lab.export(output.parent/'lab-report')
    print(f'Saved {len(rows)} live trials to {output} and {output.with_suffix(".csv")}',flush=True)

if __name__=='__main__':
    try:main()
    except subprocess.CalledProcessError as e:
        raise SystemExit('Docker experiment operation failed: '+e.stderr.decode(errors='replace')[-2000:])
