# Phase 0 tests

Actual test sources and harnesses preserved here:

- `upstream-overlay/phase0_qualification_test.go`: Go tests added by the qualification overlay for owner/private-chat policy, bounded echo, unauthorized/group rejection, and agent-bus routing.
- `../phone/test-harness.sh`: persistent on-device scenario controller.
- `../phone/collect-results.sh`: numeric PASS/FAIL/INCOMPLETE report generator.
- `../config/test-plan.json`: twenty real-phone scenarios.
- `../evidence/inspection/*selftest*`: locally executed config, redaction, supervisor, and result-collector evidence.
- `../evidence/inspection/phase0-*-tests*.txt`: scoped upstream/overlay Go test evidence.

The phone scenarios were prepared but not executed. No file in this directory claims otherwise.
