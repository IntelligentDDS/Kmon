package events

import "gitlab.dds-sysu.tech/Wuny/kmon/src/model"

type EventTick struct {
	DeltaTime uint64
}

var actionsTick map[model.Collector][]func(model.Collector, *EventTick) = map[model.Collector][]func(model.Collector, *EventTick){}
var deferTick map[*EventTick][]func(model.Collector, *EventTick) = map[*EventTick][]func(model.Collector, *EventTick){}

func ListenTick(receiver model.Collector, action func(model.Collector, *EventTick)) {
	actions, ok := actionsTick[receiver]
	if !ok {
		actions = make([]func(model.Collector, *EventTick), 0)
	}
	actionsTick[receiver] = append(actions, action)
}

func EmitTick(sender model.Collector, event *EventTick) {
	deferTick[event] = make([]func(model.Collector, *EventTick), 0)

	if actions, ok := actionsTick[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsTick[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferTick[event]
	delete(deferTick, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferTick(event *EventTick, action func(model.Collector, *EventTick)) {
	if actions, ok := deferTick[event]; ok {
		deferTick[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}
