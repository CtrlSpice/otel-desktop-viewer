"""Generate the fixed Gauge gap fixture: python3 testdata/otlp/gauge-gaps/generate.py."""

import json
import math
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parent
START = int(datetime(2026, 11, 7, 12, tzinfo=timezone.utc).timestamp()) * 1_000_000_000
SERIES = [
    ("continuous", 15, []),
    ("long-outage", 45, [(300, 480)]),
    ("short-outages", 75, [(120, 150), (600, 620), (960, 1080)]),
]

points = []
for name, level, outages in SERIES:
    for second in range(0, 1201, 5):
        if any(start <= second < end for start, end in outages):
            continue
        value = round(level + 6 * math.sin(second / 45) + 2 * math.cos(second / 17), 3)
        # Real zero readings form a continuous flat section, not an outage.
        if name == "long-outage" and 660 <= second <= 720:
            value = 0
        points.append({
            "timeUnixNano": str(START + second * 1_000_000_000),
            "asDouble": value,
            "attributes": [{"key": "host.name", "value": {"stringValue": name}}],
        })

request = {
    "resourceMetrics": [{
        "resource": {"attributes": [
            {"key": "service.name", "value": {"stringValue": "gauge-gap-demo"}},
        ]},
        "scopeMetrics": [{
            "scope": {"name": "synthetic-gauge-gaps", "version": "1.0.0"},
            "metrics": [{
                "name": "demo.gauge.gaps",
                "description": "Synthetic Gauge readings with independent host outages and a real-zero interval.",
                "unit": "1",
                "gauge": {"dataPoints": points},
            }],
        }],
    }],
}
manifest = {
    "description": "Three Gauge series: uninterrupted readings, a three-minute outage with a separate real-zero interval, and three shorter outages.",
    "startTimeUnixNano": str(START),
    "endTimeUnixNano": str(START + 1200 * 1_000_000_000),
    "requests": [{"signal": "metrics", "file": "metrics.json"}],
    "storedCounts": {"spans": 0, "logs": 0, "datapoints": len(points)},
}
for filename, data in [("metrics.json", request), ("manifest.json", manifest)]:
    (ROOT / filename).write_text(json.dumps(data, indent=2) + "\n")
