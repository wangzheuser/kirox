package core

import "reg_go/internal/browser"

func orderedExecutePayload(stepID, workflowStateHandle, actionID string, inputs []interface{}, visitorID, requestID string) *browser.OrderedMap {
	payload := browser.NewOrderedMap()
	payload.Set("stepId", stepID)
	payload.Set("workflowStateHandle", workflowStateHandle)
	if actionID != "" {
		payload.Set("actionId", actionID)
	}
	if inputs != nil {
		payload.Set("inputs", inputs)
	}
	if visitorID != "" {
		payload.Set("visitorId", visitorID)
	}
	payload.Set("requestId", requestID)
	return payload
}

func orderedUserRequestInput(username string) *browser.OrderedMap {
	input := browser.NewOrderedMap()
	input.Set("input_type", "UserRequestInput")
	input.Set("username", username)
	return input
}

func orderedApplicationTypeRequestInput(applicationType string) *browser.OrderedMap {
	input := browser.NewOrderedMap()
	input.Set("input_type", "ApplicationTypeRequestInput")
	input.Set("applicationType", applicationType)
	return input
}

func orderedUserEventRequestInput(directoryID, userName, eventType, pageName string, timeSpentOnPage int) *browser.OrderedMap {
	event := browser.NewOrderedMap()
	event.Set("input_type", "UserEvent")
	event.Set("eventType", eventType)
	event.Set("pageName", pageName)
	event.Set("timeSpentOnPage", timeSpentOnPage)

	input := browser.NewOrderedMap()
	input.Set("input_type", "UserEventRequestInput")
	input.Set("directoryId", directoryID)
	input.Set("userName", userName)
	input.Set("userEvents", []interface{}{event})
	return input
}

func orderedFingerPrintRequestInput(fp string) *browser.OrderedMap {
	input := browser.NewOrderedMap()
	input.Set("input_type", "FingerPrintRequestInput")
	input.Set("fingerPrint", fp)
	return input
}
