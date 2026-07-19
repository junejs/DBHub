#!/usr/bin/env python3
"""
Split openapi.yaml into multiple files.
Uses exact line slicing based on section boundaries.
"""
import re, os, shutil

SRC = "openapi.yaml"
BAK = SRC + ".bak"
DIR = "openapi"

os.makedirs(f"{DIR}/paths", exist_ok=True)
os.makedirs(f"{DIR}/components", exist_ok=True)

with open(SRC) as f:
    lines = f.read().split('\n')

# ── Find section boundaries (line numbers, 0-indexed) ──
boundaries = {}
for i, line in enumerate(lines):
    s = line.strip()
    if s == 'paths:':
        boundaries['paths'] = i
    elif s == 'components:':
        boundaries['components'] = i
    elif line.rstrip() == '  securitySchemes:':
        boundaries['securitySchemes'] = i
    elif line.rstrip() == '  parameters:':
        boundaries['parameters'] = i
    elif line.rstrip() == '  responses:':
        boundaries['responses'] = i
    elif line.rstrip() == '  schemas:':
        boundaries['schemas'] = i

print("Boundaries found:", {k: v+1 for k, v in boundaries.items()})

# ── Header (up to paths) ──
header = '\n'.join(lines[:boundaries['paths']])

# ── Paths section ──
path_start = boundaries['paths']
path_end = boundaries['components']
paths_text = '\n'.join(lines[path_start:path_end])

# ── Component sub-sections ──
# For each, from its line to the next sub-section start or end of file
comp_keys = ['securitySchemes', 'parameters', 'responses', 'schemas']
comp_sections = {}
for idx, key in enumerate(comp_keys):
    start = boundaries[key]
    end = boundaries[comp_keys[idx + 1]] if idx + 1 < len(comp_keys) else len(lines)
    # Also stop at the '# ═══' separator line if present
    text = '\n'.join(lines[start:end])
    # Remove the key line itself (first line)
    first_nl = text.find('\n')
    if first_nl >= 0:
        text = text[first_nl + 1:]
    # Strip leading/trailing blank lines
    comp_sections[key] = text.strip('\n')

print(f"Paths: ~{len(paths_text.split(chr(10)))} lines")
for k in comp_keys:
    print(f"  {k}: ~{len(comp_sections[k].split(chr(10)))} lines")

# ──────────────────────────────────────────────────────────
# 1. Extract PATH BLOCKS by comment headers
# ──────────────────────────────────────────────────────────
TOPIC_MAP = {
    "System": "system", "Auth": "auth", "SQL": "sql",
    "Export": "export", "Instance": "instance", "Database": "database",
    "IAM": "iam", "Audit": "audit", "Project": "project",
    "Environment": "environment", "User": "user", "Group": "group",
    "IDP": "idp", "Worksheet": "worksheet",
    "Notification": "notification", "Setting": "setting",
}

# Split paths_text into path blocks, grouped by topic comment header
path_blocks = {}  # topic -> list of path key + content
current_topic = "misc"
current_path_key = None
current_path_lines = []

def save_current_path():
    global current_path_key, current_path_lines
    if current_path_key is not None:
        topic = current_topic
        path_blocks.setdefault(topic, {})[current_path_key] = current_path_lines
        current_path_key = None
        current_path_lines = []

for line in paths_text.split('\n'):
    m = re.match(r'\s*# ─+ (\w+) ─+', line)
    if m:
        save_current_path()
        current_topic = TOPIC_MAP.get(m.group(1), m.group(1).lower())

    m = re.match(r'^  (/[\w/{}:.-]+):', line)
    if m:
        save_current_path()
        current_path_key = m.group(1)
        current_path_lines = [line]
    elif current_path_key is not None:
        current_path_lines.append(line)

save_current_path()

print(f"\nPath topics: {list(path_blocks.keys())}")
for topic, blocks in path_blocks.items():
    print(f"  {topic}: {len(blocks)} paths -> {list(blocks.keys())[:3]}...")

# ── Write path files ──
def convert_refs(text, prefix):
    """Replace '#/components/' with '{prefix}/components/' in $ref lines."""
    return re.sub(r"\$ref: '#/components/", f"$ref: '{prefix}/components/", text)

def convert_internal_refs(text):
    """For path files (at openapi/paths/): '#/components/' -> '../components.yaml#/components/'"""
    return re.sub(r"\$ref: '#/components/", f"$ref: '../components.yaml#/components/", text)

for topic, blocks in path_blocks.items():
    lines_out = []
    for path_key in sorted(blocks.keys()):
        block = blocks[path_key]
        text = convert_internal_refs('\n'.join(block))
        lines_out.append(text)
        lines_out.append('')
    content = '\n'.join(lines_out)
    fname = f"{DIR}/paths/{topic}.yaml"
    with open(fname, 'w') as f:
        f.write(content)
    print(f"  wrote {fname}")

# ── Write consolidated components.yaml ──
# Keeping all sub-sections in one file so internal $ref (#/components/...) still work
components_body = ""
for key in ['securitySchemes', 'parameters', 'responses', 'schemas']:
    content = comp_sections.get(key, '')
    components_body += f"  {key}:\n"
    # Each line of content is already at 4-space indent (after stripping the section header)
    components_body += content + "\n\n"

components_content = f"components:\n{components_body}"
with open(f"{DIR}/components.yaml", 'w') as f:
    f.write(components_content)
print(f"  wrote {DIR}/components.yaml")

# securitySchemes is small, keep inline in entry file
security_schemes = comp_sections.get('securitySchemes', '')

# ──────────────────────────────────────────────────────────
# 2. Build entry openapi.yaml
# ──────────────────────────────────────────────────────────

# Path $ref entries
path_ref_lines = []
for topic in sorted(path_blocks.keys()):
    blocks = path_blocks[topic]
    for path_key in sorted(blocks.keys()):
        pointer = path_key.replace('~', '~0').replace('/', '~1')
        path_ref_lines.append(f"  {path_key}:\n    $ref: 'openapi/paths/{topic}.yaml#/{pointer}'")

# Build component $ref section
# All components in one consolidated file at openapi/components.yaml
# Use JSON Pointer paths like /components/parameters/PageSize

# Read the components file to extract top-level names
comp_refs = {}
with open(f"{DIR}/components.yaml") as f:
    current_section = None
    for line in f:
        m = re.match(r'  (\w+):', line)
        if m and m.group(1) in ('securitySchemes', 'parameters', 'responses', 'schemas'):
            current_section = m.group(1)
            comp_refs.setdefault(current_section, [])
            continue
        m = re.match(r'    (\w+):', line)
        if m and current_section:
            comp_refs[current_section].append(m.group(1))

# Build components section with $ref to consolidated components.yaml
comp_lines = ["components:"]
comp_lines.append("")

# securitySchemes - stay inline (small)
if 'securitySchemes' in comp_refs and security_schemes:
    comp_lines.append("  securitySchemes:")
    comp_lines.append(security_schemes)
    comp_lines.append("")

# parameters
if 'parameters' in comp_refs:
    comp_lines.append("  parameters:")
    for name in sorted(comp_refs['parameters']):
        comp_lines.append(f"    {name}:")
        comp_lines.append(f"      $ref: 'openapi/components.yaml#/components/parameters/{name}'")
    comp_lines.append("")

# responses
if 'responses' in comp_refs:
    comp_lines.append("  responses:")
    for name in sorted(comp_refs['responses']):
        comp_lines.append(f"    {name}:")
        comp_lines.append(f"      $ref: 'openapi/components.yaml#/components/responses/{name}'")
    comp_lines.append("")

# schemas
if 'schemas' in comp_refs:
    comp_lines.append("  schemas:")
    for name in sorted(comp_refs['schemas']):
        comp_lines.append(f"    {name}:")
        comp_lines.append(f"      $ref: 'openapi/components.yaml#/components/schemas/{name}'")

components_refs = '\n'.join(comp_lines)

# Full entry file
entry = header + '\n\n'
entry += 'paths:\n'
for ref in path_ref_lines:
    entry += ref + '\n'
entry += '\n'
entry += components_refs + '\n'

# Backup and write
shutil.copy2(SRC, BAK)
with open(SRC, 'w') as f:
    f.write(entry)

print(f"\n✅ Backup: {BAK}")
print(f"✅ Written: {SRC}")
print(f"\nRun 'cd backend && make gen' to verify.")
