package events

import "gitlab.dds-sysu.tech/Wuny/kmon/src/model"

type EventPIDChange struct {
	AddPID []uint32
	DelPID []uint32
	AllPID []uint32
}

var actionsPIDChange map[model.Collector][]func(model.Collector, *EventPIDChange) = map[model.Collector][]func(model.Collector, *EventPIDChange){}
var deferPIDChange map[*EventPIDChange][]func(model.Collector, *EventPIDChange) = map[*EventPIDChange][]func(model.Collector, *EventPIDChange){}

func ListenPIDChange(receiver model.Collector, action func(model.Collector, *EventPIDChange)) {
	actions, ok := actionsPIDChange[receiver]
	if !ok {
		actions = make([]func(model.Collector, *EventPIDChange), 0)
	}
	actionsPIDChange[receiver] = append(actions, action)
}

func EmitPIDChange(sender model.Collector, event *EventPIDChange) {
	deferPIDChange[event] = make([]func(model.Collector, *EventPIDChange), 0)

	if actions, ok := actionsPIDChange[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsPIDChange[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferPIDChange[event]
	delete(deferPIDChange, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferPIDChange(event *EventPIDChange, action func(model.Collector, *EventPIDChange)) {
	if actions, ok := deferPIDChange[event]; ok {
		deferPIDChange[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}
