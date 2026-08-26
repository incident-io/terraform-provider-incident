package models

// Where to point people for the values of an attribute that isn't a fixed enum. The
// dashboard is the source of truth for all of these, and its "Export to Terraform" emits
// config using the exact values, so it doubles as a way to discover them.
//
// The condition and expression wording is shared by every resource built on the engine -
// escalation paths and maintenance windows as well as workflows - so keep it generic
// rather than naming the workflow builder.
const (
	WhereToFindTriggers = "Pick a trigger in the dashboard's workflow builder and use Export to " +
		"Terraform to see its name."
	WhereToFindOnceFor = "The references you can use come from the scope the trigger builds, which the " +
		"workflow builder lists once you've chosen one."
	WhereToFindOperations = "Which operations apply depends on the type of the subject you're comparing, " +
		"so a string and a timestamp offer different ones. The dashboard lists the valid ones for a subject " +
		"as you build the condition."
	WhereToFindReturnTypes = "Engine types include your own configuration - a custom field's type is " +
		"identified by its ID - so build the expression in the dashboard and export it to get the right value."
	WhereToFindFormFieldTypes = "Form field types are engine types, which include your own configuration, " +
		"so build the form in the dashboard and export it to get the right value."
)
