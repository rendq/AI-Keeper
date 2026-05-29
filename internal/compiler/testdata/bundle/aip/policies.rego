package aip

import rego.v1

# Policy: default/deny-external (effect=deny, priority=200)
aip_deny contains {"name": "default/deny-external", "priority": 200, "obligations": {}} if {
    input.principal.kind == "Anonymous"
    input.action.verb in {"invoke", "read"}
}

# Policy: default/allow-research (effect=allow, priority=100)
aip_allow contains {"name": "default/allow-research", "priority": 100, "obligations": {}} if {
    input.principal.kind == "User"; input.principal.labels["team"] == "research"
    input.action.verb in {"invoke"}
    input.action.resource.kind in {"Skill"}
}

