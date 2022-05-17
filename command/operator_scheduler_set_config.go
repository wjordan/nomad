package command

import (
	"fmt"
	"strings"

	"github.com/hashicorp/nomad/api"
	flagHelper "github.com/hashicorp/nomad/helper/flags"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
)

// Ensure OperatorSchedulerSetConfig satisfies the cli.Command interface.
var _ cli.Command = &OperatorSchedulerSetConfig{}

type OperatorSchedulerSetConfig struct {
	Meta

	// The scheduler configuration flags allow us to tell whether the user set
	// a value or not. This means we can safely merge the current configuration
	// with user supplied, selective updates.
	schedulerAlgorithm          string
	memoryOversubscription      flagHelper.BoolValue
	rejectJobRegistration       flagHelper.BoolValue
	pauseEvalBroker             flagHelper.BoolValue
	preemptionBatchScheduler    flagHelper.BoolValue
	preemptionServiceScheduler  flagHelper.BoolValue
	preemptionSysBatchScheduler flagHelper.BoolValue
	preemptionSystemScheduler   flagHelper.BoolValue
}

func (o *OperatorSchedulerSetConfig) AutocompleteFlags() complete.Flags {
	return mergeAutocompleteFlags(o.Meta.AutocompleteFlags(FlagSetClient),
		complete.Flags{
			"-scheduler-algorithm":           complete.PredictAnything,
			"-memory-oversubscription":       complete.PredictAnything,
			"-reject-job-registration":       complete.PredictAnything,
			"-pause-eval-broker":             complete.PredictAnything,
			"-preemption-batch-scheduler":    complete.PredictAnything,
			"-preemption-service-scheduler":  complete.PredictAnything,
			"-preemption-sysbatch-scheduler": complete.PredictAnything,
			"-preemption-system-scheduler":   complete.PredictAnything,
		},
	)
}

func (o *OperatorSchedulerSetConfig) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (o *OperatorSchedulerSetConfig) Name() string { return "operator scheduler set-config" }

func (o *OperatorSchedulerSetConfig) Run(args []string) int {

	flags := o.Meta.FlagSet("set-config", FlagSetClient)
	flags.Usage = func() { o.Ui.Output(o.Help()) }

	flags.StringVar(&o.schedulerAlgorithm, "scheduler-algorithm", "", "")
	flags.Var(&o.memoryOversubscription, "memory-oversubscription", "")
	flags.Var(&o.rejectJobRegistration, "reject-job-registration", "")
	flags.Var(&o.pauseEvalBroker, "pause-eval-broker", "")
	flags.Var(&o.preemptionBatchScheduler, "preemption-batch-scheduler", "")
	flags.Var(&o.preemptionServiceScheduler, "preemption-service-scheduler", "")
	flags.Var(&o.preemptionSysBatchScheduler, "preemption-sysbatch-scheduler", "")
	flags.Var(&o.preemptionSystemScheduler, "preemption-system-scheduler", "")

	if err := flags.Parse(args); err != nil {
		o.Ui.Error(fmt.Sprintf("Failed to parse args: %v", err))
		return 1
	}

	// Set up a client.
	client, err := o.Meta.Client()
	if err != nil {
		o.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}

	// Fetch the current configuration. This will be used as a base to merge
	// user configuration onto.
	resp, _, err := client.Operator().SchedulerGetConfiguration(nil)
	if err != nil {
		o.Ui.Error(fmt.Sprintf("Error querying for Autopilot configuration: %s", err))
		return 1
	}
	schedulerConfig := resp.SchedulerConfig

	// Merge the current configuration with any values set by the operator.
	if o.schedulerAlgorithm != "" {
		schedulerConfig.SchedulerAlgorithm = api.SchedulerAlgorithm(o.schedulerAlgorithm)
	}
	o.memoryOversubscription.Merge(&schedulerConfig.MemoryOversubscriptionEnabled)
	o.rejectJobRegistration.Merge(&schedulerConfig.RejectJobRegistration)
	o.pauseEvalBroker.Merge(&schedulerConfig.PauseEvalBroker)
	o.preemptionBatchScheduler.Merge(&schedulerConfig.PreemptionConfig.BatchSchedulerEnabled)
	o.preemptionServiceScheduler.Merge(&schedulerConfig.PreemptionConfig.ServiceSchedulerEnabled)
	o.preemptionSysBatchScheduler.Merge(&schedulerConfig.PreemptionConfig.SysBatchSchedulerEnabled)
	o.preemptionSystemScheduler.Merge(&schedulerConfig.PreemptionConfig.SystemSchedulerEnabled)

	// Check-and-set the new configuration.
	result, _, err := client.Operator().SchedulerCASConfiguration(schedulerConfig, nil)
	if err != nil {
		o.Ui.Error(fmt.Sprintf("Error setting scheduler configuration: %s", err))
		return 1
	}
	if result.Updated {
		o.Ui.Output("Scheduler configuration updated!")
		return 0
	}
	o.Ui.Output("Scheduler configuration could not be atomically updated, please try again")
	return 1
}

func (o *OperatorSchedulerSetConfig) Synopsis() string {
	return "Modify the current scheduler configuration"
}

func (o *OperatorSchedulerSetConfig) Help() string {
	helpText := `
Usage: nomad operator scheduler set-config [options]

  Modifies the current scheduler configuration.

  If ACLs are enabled, this command requires a token with the 'operator:write'
  capability.

General Options:

  ` + generalOptionsUsage(usageOptsDefault|usageOptsNoNamespace) + `

Scheduler Set Config Options:

  -scheduler-algorithm=["binpack"|"spread"]
    Specifies whether scheduler binpacks or spreads allocations on available
    nodes.

  -memory-oversubscription=[true|false]
    When true, tasks may exceed their reserved memory limit, if the client has
    excess memory capacity. Tasks must specify memory_max to take advantage of
    memory oversubscription.

  -reject-job-registration=[true|false]
    When true, the server will return permission denied errors for job registration,
    job dispatch, and job scale APIs, unless the ACL token for the request is a
    management token. If ACLs are disabled, no user will be able to register jobs.
    This allows operators to shed load from automated processes during incident
    response.

  -pause-eval-broker=[true|false]
    When set to true, the eval broker which usually runs on the leader will be
    disabled. This will prevent the scheduler workers from receiving new work.

  -preemption-batch-scheduler=[true|false]
    Specifies whether preemption for batch jobs is enabled. Note that if this
    is set to true, then batch jobs can preempt any other jobs.

  -preemption-service-scheduler=[true|false]
    Specifies whether preemption for service jobs is enabled. Note that if this
    is set to true, then service jobs can preempt any other jobs.

  -preemption-sysbatch-scheduler=[true|false]
    Specifies whether preemption for system batch jobs is enabled. Note that if
    this is set to true, then system batch jobs can preempt any other jobs.

  -preemption-system-scheduler=[true|false]
    Specifies whether preemption for system jobs is enabled. Note that if this
    is set to true, then system jobs can preempt any other jobs.
`
	return strings.TrimSpace(helpText)
}
