package nsenrol

// Plan is what a namespace's enrolment is, and what would fix it.
type Plan struct {
	State State
	// Create are the bindings to create.
	Create []Binding
	// Replace are the bindings to delete first, because a roleRef is immutable
	// and there is no editing one into the right shape.
	Replace []string
}

// Done reports that there is nothing to do.
func (p Plan) Done() bool { return len(p.Create) == 0 && len(p.Replace) == 0 }

// Decide compares what a namespace has against what enrolment means.
//
// This is the whole of the logic and it does no I/O, which is what makes it the
// piece worth testing: given the bindings actually present, it answers what the
// namespace's state is and what would fix it.
//
// A binding that is present but wrong is replaced rather than patched. roleRef is
// immutable — Kubernetes refuses the update — so anything that tried to edit one
// would fail forever on exactly the namespaces that most need fixing.
func (c Config) Decide(live []Binding) Plan {
	plan := Plan{State: StateEnrolled}

	for _, want := range c.wanted() {
		found, present := find(live, want.name)
		switch {
		case !present:
			plan.Create = append(plan.Create, c.binding(want))
		case want.matches(found):
		default:
			plan.Replace = append(plan.Replace, want.name)
			plan.Create = append(plan.Create, c.binding(want))
		}
	}

	switch {
	case len(plan.Replace) > 0:
		// Wrong wins over partial. A binding that grants the wrong thing is worse
		// than one that is absent — absent fails a deploy loudly, and wrong can
		// succeed at something nobody asked for.
		plan.State = StateWrong
	case len(plan.Create) == len(c.wanted()):
		plan.State = StateMissing
	case len(plan.Create) > 0:
		plan.State = StatePartial
	default:
		plan.State = StateEnrolled
	}
	return plan
}

// binding renders a wanted binding as the object to create.
func (c Config) binding(want wanted) Binding {
	return Binding{
		Name:        want.name,
		RoleRefKind: "ClusterRole",
		RoleRef:     want.role,
		Subjects:    []string{want.subject},
	}
}

// find returns the live binding of that name, if it is there.
func find(live []Binding, name string) (Binding, bool) {
	for _, one := range live {
		if one.Name == name {
			return one, true
		}
	}
	return Binding{}, false
}
