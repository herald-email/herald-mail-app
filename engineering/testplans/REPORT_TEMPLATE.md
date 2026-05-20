# Herald Test Report Template

Use this template for durable reports saved under `reports/`. Keep reports short, evidence-backed, and explicit about which surface actually proved the change.

## Summary

- Change under test:
- Result:
- Remaining risk:

## Verification Surface

Mark every surface exercised. Leave unchecked surfaces visible so readers can see what was not proven.

- [ ] `demo`
- [ ] `virtual lab`
- [ ] `live config`
- [ ] `tmux`
- [ ] `ttyd`
- [ ] `SSH`
- [ ] `MCP`
- [ ] `daemon`

## Checks Run

| Check | Command or Scenario | Result | Evidence |
|-------|---------------------|--------|----------|
| Focused test |  |  |  |
| Package test |  |  |  |
| Surface check |  |  |  |

## Degradation Check

- Preserved behaviors:
- Approved degradations, if any:
- Regression checks that protect nearby behavior:

## Failure Notes

If a command failed twice, record the second-failure rule notes before continuing:

- Hypothesis:
- Smallest failing command:
- Failure class:
- Next narrower diagnostic:
