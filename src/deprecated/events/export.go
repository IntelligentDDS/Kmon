package events

import "gitlab.dds-sysu.tech/Wuny/kmon/src/model"

type EventExport struct {
	Data      []*model.ExportData
	Exporters map[string]string
}

var actionsExport map[model.Collector][]func(model.Collector, *EventExport) = map[model.Collector][]func(model.Collector, *EventExport){}
var deferExport map[*EventExport][]func(model.Collector, *EventExport) = map[*EventExport][]func(model.Collector, *EventExport){}

func ListenExport(receiver model.Collector, action func(model.Collector, *EventExport)) {
	actions, ok := actionsExport[receiver]
	if !ok {
		actions = make([]func(model.Collector, *EventExport), 0)
	}
	actionsExport[receiver] = append(actions, action)
}

func EmitExport(sender model.Collector, event *EventExport) {
	deferExport[event] = make([]func(model.Collector, *EventExport), 0)

	if actions, ok := actionsExport[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsExport[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferExport[event]
	delete(deferExport, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferExport(event *EventExport, action func(model.Collector, *EventExport)) {
	if actions, ok := deferExport[event]; ok {
		deferExport[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}
