package events

import "gitlab.dds-sysu.tech/Wuny/kmon/src/model"

type EventPeriod struct {
	DeltaTime uint64
}

var actionsPeriod map[model.Collector][]func(model.Collector, *EventPeriod) = map[model.Collector][]func(model.Collector, *EventPeriod){}
var deferPeriod map[*EventPeriod][]func(model.Collector, *EventPeriod) = map[*EventPeriod][]func(model.Collector, *EventPeriod){}

func ListenPeriod(receiver model.Collector, action func(model.Collector, *EventPeriod)) {
	actions, ok := actionsPeriod[receiver]
	if !ok {
		actions = make([]func(model.Collector, *EventPeriod), 0)
	}
	actionsPeriod[receiver] = append(actions, action)
}

func EmitPeriod(sender model.Collector, event *EventPeriod) {
	deferPeriod[event] = make([]func(model.Collector, *EventPeriod), 0)

	if actions, ok := actionsPeriod[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsPeriod[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferPeriod[event]
	delete(deferPeriod, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferPeriod(event *EventPeriod, action func(model.Collector, *EventPeriod)) {
	if actions, ok := deferPeriod[event]; ok {
		deferPeriod[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}
