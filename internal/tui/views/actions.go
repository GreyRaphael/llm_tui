package views

// Action identifies a navigation or lifecycle event emitted by a view model.
type Action string

const (
	ActionNone          Action = ""
	ActionProbeNew      Action = "probe_new"
	ActionOpenTester    Action = "open_tester"
	ActionBackToManager Action = "back_to_manager"
	ActionRecordCreated Action = "record_created"
)
