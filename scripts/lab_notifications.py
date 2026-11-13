"""Exercise real alert/outbox/HTTPS delivery on the owned Compose stand.

Only fault controls and raw observations are recorded. No DSN or bearer data
is exported. Leaves the stand healthy and preserves PostgreSQL evidence.
"""
import argparse
import json
import secrets
import time
from datetime import datetime, timezone
from pathlib import Path
from lab import Lab

def run(lab):
    lab.require()
    lab.healthy()
    before = int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))
    started = datetime.now(timezone.utc).isoformat()
    def alert():
        data = lab.snapshot('alert.json')
        return json.loads(data) if data else {}
    def fault(count):
        code = "import os,sys;p='/data/mode.json';f=p+'.pending';open(f,'wb').write(sys.stdin.buffer.read());os.replace(f,p)"
        lab.compose('exec','-T','webhook','python3','-c',code,
                    input=json.dumps({'generation':secrets.token_hex(8),'fail_next':count}).encode())
    def outbox(kind):
        text = lab.sql("SELECT row_to_json(t) FROM (SELECT event_id,state,attempts,status_code,payload FROM nmo_webhook_outbox WHERE payload->>'occurred_at'>='"+started+"' AND payload->>'event_type'='alert."+kind+"' ORDER BY next_after DESC LIMIT 1)t")
        return json.loads(text) if text else None
    def delivered(kind):
        job = outbox(kind)
        return job if job and job['state']=='delivered' else None
    def attempts(event):
        return json.loads(lab.sql("SELECT coalesce(json_agg(t ORDER BY attempt),'[]') FROM (SELECT attempt,status_code,finished_at IS NOT NULL AS finished FROM nmo_webhook_attempts WHERE event_id='"+event+"')t"))

    # Force a real 503, then successful delivery to the same HTTPS endpoint.
    fault(1)
    lab.inject('http')
    lab.wait(lambda:alert().get('status')=='firing','pending -> firing')
    first = lab.wait(lambda:delivered('firing'),'503 -> retry -> 200')
    first_attempts = attempts(first['event_id'])
    assert [a['status_code'] for a in first_attempts]==[503,200],first_attempts

    # An Agent outage removes coverage but must not resolve active firing.
    lab.compose('stop','branch','dc')
    try:
        lab.wait(lambda:lab.assessment().get('state')=='unknown','stale evidence -> unknown',seconds=40)
        assert alert()['status']=='firing','unknown resolved an active alert'
        previous_tick = alert()['lastEvaluatedAt']
        lab.compose('restart','controller')
        lab.wait(lambda:alert().get('lastEvaluatedAt','')>previous_tick and alert().get('status')=='firing','restore firing')
        time.sleep(3)
        assert int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))==before+1,'restart emitted duplicate firing'
    finally:
        lab.compose('start','branch','dc')

    # Queue a resolution while receiver returns errors; restart Controller
    # after a committed failed attempt. Retrying must preserve event identity.
    fault(100)
    lab.healthy()
    lab.wait(lambda:alert().get('status')=='resolved','explicit healthy resolves')
    pending = lab.wait(lambda:(j if (j:=outbox('resolved')) and j['attempts']>=1 else None),'durable resolution attempt')
    lab.compose('stop','controller')
    fault(0)
    lab.compose('start','controller')
    resolution = lab.wait(lambda:delivered('resolved'),'restart resumes resolution delivery',seconds=30)
    assert resolution['event_id']==pending['event_id'],'event id changed on retry'
    assert resolution['attempts']>=2 and resolution['status_code']==200,resolution
    assert int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))==before+2,'unexpected queued replay'
    def report_delivered():
        raw=lab.snapshot('report.html')
        html=raw.decode() if raw else ''
        return resolution['event_id'] in html and 'Стан доставки: <strong>delivered</strong>' in html
    lab.wait(report_delivered,'HTML reflects committed delivery, not just enqueue',seconds=10)
    # Receiver-side database proves logical dedup without recording secrets.
    code = "import sqlite3,json;d=sqlite3.connect('/data/receiver.sqlite');print(json.dumps({'events':d.execute('SELECT id,payload FROM events WHERE id IN (?,?)',json.loads(__import__('sys').stdin.read())).fetchall(),'attempts':d.execute('SELECT event_id,status FROM attempts ORDER BY seq').fetchall()}))"
    receiver = json.loads(lab.compose('exec','-T','webhook','python3','-c',code,input=json.dumps([first['event_id'],resolution['event_id']]).encode()))
    assert len(receiver['events'])==2,'receiver logical dedup failed'
    lab.healthy()
    return {'schema':1,'startedAt':started,'finishedAt':datetime.now(timezone.utc).isoformat(),
            'project':lab.project,'firing':first,'firingAttempts':first_attempts,
            'resolution':resolution,'resolutionAttempts':attempts(resolution['event_id']),
            'receiver':receiver,'unknownHeldFiring':True,'restartKeptEventID':True,'htmlDeliveryVerified':True,
            'finalAssessment':lab.assessment(),'finalAlert':alert()}

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project',default='netmonorc-lab')
    parser.add_argument('--prefix',default='172.30')
    parser.add_argument('--output',default='.local/notifications.json')
    args=parser.parse_args()
    result=run(Lab(args.project,args.prefix))
    p=Path(args.output);p.parent.mkdir(parents=True,exist_ok=True)
    p.write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n');p.chmod(0o600)
    print('Real HTTPS delivery, durable retries, unknown-state hold and restart recovery verified:',p)
