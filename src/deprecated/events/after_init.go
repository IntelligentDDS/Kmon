package events

import "gitlab.dds-sysu.tech/Wuny/kmon/src/model"

type EventAfterInit struct{}

var actionsAfterInit map[model.Collector][]func(model.Collector, *EventAfterInit) = map[model.Collector][]func(model.Collector, *EventAfterInit){}
var deferAfterInit map[*EventAfterInit][]func(model.Collector, *EventAfterInit) = map[*EventAfterInit][]func(model.Collector, *EventAfterInit){}

func ListenAfterInit(receiver model.Collector, action func(model.Collector, *EventAfterInit)) {
	actions, ok := actionsAfterInit[receiver]
	if !ok {
		actions = make([]func(model.Collector, *EventAfterInit), 0)
	}
	actionsAfterInit[receiver] = append(actions, action)
}

func EmitAfterInit(sender model.Collector, event *EventAfterInit) {
	deferAfterInit[event] = make([]func(model.Collector, *EventAfterInit), 0)

	if actions, ok := actionsAfterInit[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsAfterInit[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferAfterInit[event]
	delete(deferAfterInit, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferAfterInit(event *EventAfterInit, action func(model.Collector, *EventAfterInit)) {
	if actions, ok := deferAfterInit[event]; ok {
		deferAfterInit[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}
