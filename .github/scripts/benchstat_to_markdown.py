#!/usr/bin/env python3
"""Convert benchstat text output to a markdown table."""

import re
import sys

input_file = sys.argv[1] if len(sys.argv) > 1 else 'benchstat.txt'
output_file = sys.argv[2] if len(sys.argv) > 2 else 'benchstat.md'

text = open(input_file).read()
meta, sections, current = [], [], None

for line in text.split('\n'):
    if not line.strip():
        if current:
            sections.append(current)
            current = None
        continue
    if '│' in line:
        if current is None:
            current = {'pipes': [], 'data': [], 'notes': []}
        current['pipes'].append(line)
    elif current:
        if re.match(r'^[¹²³⁴⁵]', line.strip()):
            current['notes'].append(line.strip())
        else:
            current['data'].append(line)
    else:
        meta.append(line.strip())

if current:
    sections.append(current)


def parse_row(line):
    return [p for p in re.split(r'\s{2,}', line.strip()) if p]


md = '## Benchmark Results\n\n'

if meta:
    md += '\n'.join('> ' + l for l in meta) + '\n\n'

for s in sections:
    if len(s['pipes']) < 2:
        continue
    metric = [p.strip() for p in s['pipes'][1].split('│') if p.strip()][0]
    md += f'#### {metric}\n\n'
    md += '| Benchmark | Baseline | Incoming | Delta |\n'
    md += '|:----------|:--------:|:--------:|:-----:|\n'
    for row in s['data']:
        parts = parse_row(row)
        if not parts:
            continue
        name = parts[0]
        baseline = parts[1] if len(parts) > 1 else ''
        incoming = parts[2] if len(parts) > 2 else ''
        delta = parts[3] if len(parts) > 3 else ''
        if name == 'geomean':
            name, baseline, incoming, delta = (
                f'**{name}**', f'**{baseline}**', f'**{incoming}**', f'**{delta}**'
            )
        md += f'| {name} | {baseline} | {incoming} | {delta} |\n'
    md += '\n'
    if s['notes']:
        md += '  \n'.join(f'_{n}_' for n in s['notes']) + '\n\n'

open(output_file, 'w').write(md)
