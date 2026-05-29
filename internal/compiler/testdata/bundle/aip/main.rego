package aip

import rego.v1

# Decision algorithm: higher priority wins; at same priority deny wins.
# This is the entry point for policy evaluation.

default allow := false

# allow is true when there is at least one allow decision at the highest
# matching priority and no deny at that same priority or higher.
allow if {
    some decision in allow_decisions
    not denied_at_or_above(decision.priority)
}

# denied_at_or_above checks if there is a deny decision at the given
# priority or higher.
denied_at_or_above(priority) if {
    some d in deny_decisions
    d.priority >= priority
}

# Collect all allow decisions from individual policy rules.
allow_decisions contains decision if {
    some decision in data.aip.aip_allow
}

# Collect all deny decisions from individual policy rules.
deny_decisions contains decision if {
    some decision in data.aip.aip_deny
}

# The final decision object.
decision := {
    "allow": allow,
    "deny": count(deny_decisions) > 0,
    "matched_policies": matched_policies,
}

matched_policies contains name if {
    some d in allow_decisions
    name := d.name
}

matched_policies contains name if {
    some d in deny_decisions
    name := d.name
}
