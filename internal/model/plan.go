package model

type ScenarioPlan struct {
	ScenarioName string
	Profiles     []ProfilePlan
}

type ProfilePlan struct {
	Name      string
	Input     ResolvedInput
	Output    ResolvedOutput
	Mutations []ResolvedMutation
}

type ResolvedInput struct {
	Name     string
	Kind     string
	Codec    Codec
	Location string
	Options  map[string]string
}

type ResolvedOutput struct {
	Name    string
	Kind    string
	Target  string
	Options map[string]string
}

type ResolvedMutation struct {
	Name    string
	Kind    string
	Enabled bool
	Options map[string]string
}
