#!/usr/bin/env python3
"""Render the public dependency register from records and authoritative pins."""
import argparse
from collections import Counter
from html import escape
import json
from pathlib import Path
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'docs/capabilities/dependencies/index.html'
PUBLIC = 'https://github.com/type-rb/type-rb/blob/main/'


def render():
    data = json.loads((ROOT / 'docs/native-dependencies.json').read_text())
    assert data['schemaVersion'] == 1
    pins = json.loads((ROOT / 'tools/network-dependencies.json').read_text())
    pins.update(json.loads((ROOT / 'tools/compression-dependencies.json').read_text()))
    pins.update(json.loads((ROOT / 'tools/crypto-dependencies.json').read_text()))
    pins.update(json.loads((ROOT / 'tools/oidc-dependencies.json').read_text()))
    pins.update(json.loads((ROOT / 'tools/sqlite-dependencies.json').read_text()))
    qbe = json.loads((ROOT / 'tools/qbe/backend.json').read_text())
    pins['qbe'] = {'version': qbe['version'] + ' / ' + qbe['variant'], 'sha256': qbe['sourceSHA256']}
    rows, ids = [], set()
    for item in data['dependencies']:
        assert item['id'] not in ids and item['status'] in ('current', 'candidate'), item['id']
        ids.add(item['id'])
        assert item['decision'] in ('retain', 'selected', 'evaluate', 'deferred'), item['id']
        assert isinstance(item['requiredBy'], list) and item['requiredBy'] and all(isinstance(x, str) and x.strip() for x in item['requiredBy']), item['id']
        for key in ('name', 'scope', 'role', 'configuration', 'next', 'referenceBehavior'):
            assert isinstance(item[key], str) and item[key].strip(), (item['id'], key)
        links = []
        for source in item['evidence']:
            if source.startswith('https://'):
                url, label = source, urlparse(source).netloc
            else:
                path = ROOT / source
                assert '..' not in Path(source).parts and not Path(source).is_absolute() and path.is_file(), source
                url, label = PUBLIC + source, Path(source).name
            links.append(f'<a href="{escape(url, quote=True)}">{escape(label)}</a>')
        assert links, item['id']
        version = ''
        if 'pin' in item:
            assert item['status'] == 'current' and item['pin'] in pins
            pin = pins[item['pin']]
            version = f'<strong class="version">{escape(pin["version"])}</strong><details><summary>Source SHA-256</summary><code>{escape(pin["sha256"])}</code></details>'
            commit = pin.get('commit', pin.get('revision'))
            if commit:
                version += f'<details><summary>Upstream commit</summary><code>{escape(commit)}</code></details>'
        status = 'In source' if item['status'] == 'current' else 'Candidate'
        rows.append(f'''<tr data-status="{item['status']}" id="{escape(item['id'], quote=True)}">
<th scope="row">{escape(item['name'])}<span class="state {item['status']}">{status}</span></th>
<td><span class="scope">{escape(item['scope'])}</span><p>{escape(item['role'])}</p><p><strong>Required by:</strong> {escape(', '.join(item['requiredBy']))}</p></td>
<td>{version}<p>{escape(item['configuration'])}</p><p><strong>Decision:</strong> {escape(item['decision'])} (separate from source status)</p><details><summary>Reference behavior</summary><p>{escape(item['referenceBehavior'])}</p></details></td>
<td><div class="evidence">{' · '.join(links)}</div><p>{escape(item['next'])}</p></td></tr>''')
    counts = Counter(item['status'] for item in data['dependencies'])
    return '''<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="description" content="TypeRB Native runtime, data and build dependencies, exact source pins and candidates not yet adopted.">
<title>Dependencies · TypeRB Native</title><link rel="stylesheet" href="../styles.css"><link rel="stylesheet" href="./style.css"></head>
<body><header class="site-header"><div class="shell header-inner">
<a class="brand" href="../"><span class="brand-mark" aria-hidden="true">TR</span><span>TypeRB Native</span><span class="development">Development</span></a>
<nav class="site-nav" aria-label="Site navigation"><a href="../">Home</a><a href="../benchmarks/application/">Benchmarks</a><a href="./" aria-current="page">Dependencies</a></nav></div></header>
<main class="shell dependency-page"><p class="eyebrow">RUNTIME · SYSTEM DATA · TOOLCHAIN</p><h1>What does Native depend on?</h1>
<p class="lead">The current source uses platform services and a small set of external tools and libraries. Candidates below, including selected implementation directions, are not installed dependencies. Build tools, runtime libraries and system data have different distribution costs.</p>
<p>Read the <a href="https://github.com/type-rb/type-rb/blob/main/docs/native-dependency-policy.md">v0.5 linking, trust and distribution policy</a>. Adoption decisions require separate implementation and artifact qualification.</p>
<div class="summary"><p><strong>''' + str(counts['current']) + '''</strong> current entries</p><p><strong>''' + str(counts['candidate']) + '''</strong> candidates</p><p class="summary-note">Source status is not a supported-release claim. Artifact receipts identify the actual linked libraries, versions and licenses on each target.</p></div>
<section aria-labelledby="register-title"><h2 id="register-title">Dependency register</h2>
<div class="controls"><label>Show <select id="status"><option value="all">All components</option><option value="current">In source</option><option value="candidate">Candidates only</option></select></label><label>Search <input type="search" id="search" placeholder="Name, role, platform…"></label><span id="visible" role="status"></span></div>
<div class="table-scroll" tabindex="0" aria-label="Scrollable dependency table"><table><thead><tr><th scope="col">Component</th><th scope="col">Where and why</th><th scope="col">Version and configuration</th><th scope="col">Evidence and update requirements</th></tr></thead><tbody>
''' + '\n'.join(rows) + '''
</tbody></table></div></section>
<div class="notes"><section><h2>Use mature foundations first</h2><p>Keep language semantics, typed values, routing, ownership and errors in TypeRB. Use existing implementations for transport, TLS and database engines behind narrow owned adapters. A future replacement needs measured application benefit, conformance evidence and a sustainable maintenance cost.</p><p>TypeRB already owns managed allocation and GC, JSON values, Hex/Base64, routing and terminal editing. Existing OS/libc dependencies still apply. Focus future optimization on those TypeRB-aware boundaries before replacing security protocols or storage engines.</p></section>
<section><h2>Read dependencies in context</h2><p>The <code>trb/web</code> provider currently links libuv and llhttp, including imports using only Web response values. Importing its compression middleware also links libdeflate. The compiler and programs using only <code>trb/http</code> values need none of these transport/compression archives. Digest, HMAC and secure comparison select static libcrypto; the full CLI also links it for REPL, while the core compiler and lockfile checksum stay independent. Compiled applications need no runtime library-directory override. The HTTP provider is currently HTTP/1; this does not imply TLS, HTTP/2, streaming or a general task scheduler.</p><p>OIDC Bearer applications additionally select static libcurl, libssl, zlib, nghttp2 and ICU common/UTS46 data, sharing the existing libcrypto archive. ICU also uses the platform C++ runtime. OIDC HTTPS negotiates HTTP/2 with HTTP/1.1 fallback and applies the common Unicode 17 hostname policy. These client dependencies do not change the HTTP/1 server profile. The bootstrap <code>curl</code> command is a separate build dependency. Client HTTPS support does not add server TLS. HTTP/2, DNS, IDN, authentication and TLS backends can add conditional dependencies; check the <a href="https://curl.se/docs/libs.html">selected library configuration</a>. Do not count all optional backends as mandatory.</p></section>
<section><h2>Keep the register tied to code</h2><p>Update <a href="https://github.com/type-rb/type-rb/blob/main/docs/native-dependencies.json">the canonical records</a> and source pins in the implementation PR. Generate this page with <code>python3 tools/dependency-pages.py --write</code>. CI checks the exact generated view before deployment.</p><p>For each dependency update, verify its source hash, ABI, build flags, notices, target tests and actual linked-library receipt. Record unresolved adoption or distribution choices as candidates. This register is a readable project inventory; it does not replace a complete per-artifact software bill of materials.</p></section></div>
<footer><p>TypeRB Native · <a href="https://github.com/type-rb/type-rb/blob/main/docs/native-network.md">HTTP transport contract</a> · public source and upstream documentation</p></footer></main><script src="./app.js" defer></script></body></html>
'''


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--write', action='store_true')
    group.add_argument('--check', action='store_true')
    args = parser.parse_args()
    content = render()
    if args.write:
        OUT.parent.mkdir(parents=True, exist_ok=True)
        OUT.write_text(content)
    elif not OUT.exists() or OUT.read_text() != content:
        raise SystemExit('dependency page is stale; run tools/dependency-pages.py --write')
    print('dependency register: records, source pins and public view agree')
