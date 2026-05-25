package rules

import (
	"context"
	"fmt"

	"github.com/herald-email/herald-mail-app/internal/models"
	"github.com/herald-email/herald-mail-app/internal/work"
)

type AutomationLane struct {
	engine      *Engine
	coordinator *work.Coordinator
}

type AutomationResult struct {
	result work.Result
}

func NewAutomationLane(engine *Engine) *AutomationLane {
	return &AutomationLane{
		engine:      engine,
		coordinator: work.NewCoordinator(),
	}
}

func (l *AutomationLane) Close() {
	if l == nil || l.coordinator == nil {
		return
	}
	l.coordinator.Close()
}

func (l *AutomationLane) Submit(ctx context.Context, event models.AutomationEvent) AutomationResult {
	if l == nil || l.coordinator == nil {
		return AutomationResult{result: work.Result{}}
	}
	event = event.WithDefaults()
	result := l.coordinator.Submit(ctx, work.Spec{
		SourceID: work.SourceID(event.SourceID),
		ResourceKey: work.ResourceKey{
			SourceID:     string(event.SourceID),
			AccountID:    string(event.AccountID),
			CollectionID: event.Collection.CollectionID,
			ItemID:       event.ItemID(),
			Operation:    string(event.Kind),
		},
		Policy:   work.PolicySerialBySource,
		Priority: work.PriorityBackground,
		Run: func(context.Context) (any, error) {
			return l.evaluate(event)
		},
	})
	return AutomationResult{result: result}
}

func (r AutomationResult) Await(ctx context.Context) (models.RuleResult, error) {
	value, err := r.result.Await(ctx)
	if ruleResult, ok := value.(models.RuleResult); ok {
		if err != nil {
			ruleResult.Err = err
		}
		return ruleResult, err
	}
	if err == nil {
		err = fmt.Errorf("automation result missing")
	}
	return models.RuleResult{Err: err}, err
}

func (l *AutomationLane) evaluate(event models.AutomationEvent) (models.RuleResult, error) {
	event = event.WithDefaults()
	result := models.RuleResult{
		Kind:      event.Kind,
		SourceID:  event.SourceID,
		AccountID: event.AccountID,
		ItemID:    event.ItemID(),
	}

	switch event.Kind {
	case models.AutomationEventMailMessageReceived:
		if event.Email == nil {
			err := fmt.Errorf("mail automation event missing email")
			result.Err = err
			return result, err
		}
		result.MessageID = event.Email.MessageID
		fired, err := l.engine.EvaluateEmail(event.Email, event.Category)
		result.FiredCount = fired
		result.Err = err
		return result, err
	case models.AutomationEventCalendarEventChanged:
		result.EventID = event.EventRef.EventID
		return result, nil
	default:
		err := fmt.Errorf("unknown automation event kind: %s", event.Kind)
		result.Err = err
		return result, err
	}
}
