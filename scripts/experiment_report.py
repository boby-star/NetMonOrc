"""Generate a descriptive, auditable Markdown report from live experiment JSON.

No external libraries, invented measurements or significance claims. Harness
errors remain in the denominator; missing latency stays missing.
"""
import argparse
import json
from pathlib import Path
import statistics


def report(data):
    if data.get('metadata', {}).get('source') != 'live-compose-fault-injection':
        raise ValueError('requires live Compose experiment evidence')
    rows = data.get('trials', [])
    if not rows:
        raise ValueError('no trials')
    keys = [(r['repetition'], r['mode'], r['scenario']) for r in rows]
    if len(set(keys)) != len(keys):
        raise ValueError('duplicate trial identity')
    meta = data['metadata']
    expected = meta['repetitions'] * len(meta['modes']) * len(meta['scenarios'])
    lines = ['# Описові результати натурних експериментів', '',
             f"Джерело: `{meta['git_head']}`. Отримано {len(rows)} із {expected} запланованих запусків.",
             'Робоче дерево на початку: ' + ('зі змінами.' if meta['git_dirty'] else 'чисте.'), '',
             'Частки нижче — емпіричні результати цього стенда. Час — медіана [мінімум; максимум] у мс.',
             'Пропущені вимірювання не замінюються нулями. Помилки стенда враховані окремо.', '',
             '| Режим | Сценарій | Запуски | Правильний стан | Очікуваний діагностичний результат | Inconclusive | Хибний визначений клас | Помилки стенда | Час | Фізичні завершені probes |',
             '|---|---|---:|---:|---:|---:|---:|---:|---|---|']
    for mode in meta['modes']:
        for scenario in meta['scenarios']:
            group = [r for r in rows if r['mode'] == mode and r['scenario'] == scenario]
            def spread(field):
                values = [r[field] for r in group if isinstance(r.get(field), (int, float)) and not isinstance(r[field], bool)]
                if any(v < 0 for v in values):
                    raise ValueError('negative measurement')
                return f'{statistics.median(values):g} [{min(values):g}; {max(values):g}] (n={len(values)})' if values else 'немає даних'
            good_state = sum(r.get('state_correct') is True for r in group)
            good_diagnosis = sum(r.get('diagnosis_correct') is True for r in group)
            inconclusive = sum(r.get('status') == 'inconclusive' for r in group)
            wrong = sum(r.get('status') == 'completed' and r.get('diagnosis_correct') is False for r in group)
            errors = sum(r.get('status') == 'harness_error' for r in group)
            lines.append(f'| {mode} | {scenario} | {len(group)} | {good_state} | {good_diagnosis} | {inconclusive} | {wrong} | {errors} | {spread("duration_ms")} | {spread("completed_probes")} |')
    lines += ['', 'Для multiple очікуваним результатом є inconclusive; це не правильна класифікація конкретної причини.',
              'Час Engine.Run не включає затримку до trigger. Probes і costUnits не є виміряними байтами.',
              'Зони логічно розділені, але мають спільний хост. Повторення не доводять незалежності спостережень.',
              'Ці дані не доводять універсальної переваги Adaptive або статистичної значущості різниці часу.', '']
    return '\n'.join(lines)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('input')
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    text = report(json.loads(Path(args.input).read_text()))
    destination = Path(args.output)
    if destination.exists():
        parser.error('output already exists; preserve earlier reports')
    destination.write_text(text)
    destination.chmod(0o600)
