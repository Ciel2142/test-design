## Complete example

Given a profile whose `walkthrough` list declares `["echo", "live-fixture-ready"]` and
`["printenv", "LIVE_FIXTURE_CANARY"]`, whose `cleanup` list declares `["rm", "-f", …]`, no
declared endpoints, and applicable rows SC-11 (smoke, happy path) and SC-12 (e2e,
non-happy path), with SC-13 in the layer-1 gap set:

```json
{
  "schemaVersion": "1",
  "steps": [
    {
      "expectation": "the happy walkthrough reaches the ready marker",
      "command": ["echo", "live-fixture-ready"],
      "predicates": [
        {"form": "exit-code", "value": "0"},
        {"form": "matches", "stream": "stdout", "value": "live-fixture-ready"}
      ],
      "scBindings": ["SC-11"]
    },
    {
      "expectation": "the non-happy walkthrough is refused",
      "command": ["echo", "live-fixture-accepted"],
      "predicates": [
        {"form": "matches", "stream": "stdout", "value": "live-fixture-refused"}
      ],
      "scBindings": ["SC-12"]
    },
    {
      "expectation": "the walkthrough process cannot read a non-allowlisted secret",
      "command": ["printenv", "LIVE_FIXTURE_CANARY"],
      "predicates": [
        {"form": "exit-code", "value": "1"}
      ],
      "scBindings": ["SC-11"]
    }
  ],
  "cleanup": [
    {
      "expectation": "the walkthrough's own marker file is removed",
      "command": ["rm", "-f", "walkthrough-marker.txt"],
      "predicates": [
        {"form": "exit-code", "value": "0"}
      ],
      "scBindings": []
    }
  ]
}
```

SC-13 does not appear in any `scBindings` because it is in the gap set; every other
applicable row (SC-11, SC-12) is bound at least once; every command's first token
(`echo`, `printenv`, `rm`) is one the profile declares; no endpoint tokens appear, so
nothing needs to match `endpoints`; every predicate is `exit-code` or `matches`.
