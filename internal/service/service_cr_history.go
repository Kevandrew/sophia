package service

import (
	"errors"
	"fmt"
	"strings"

	"sophia/internal/model"
)

func (s *Service) RedactCRNote(id, noteIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return guardErr
	}

	idx, err := oneBasedIndex(noteIndex, len(cr.Notes), "note")
	if err != nil {
		return err
	}
	if cr.Notes[idx] == redactedPlaceholder {
		return ErrAlreadyRedacted
	}
	cr.Notes[idx] = redactedPlaceholder

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            model.EventTypeCRRedacted,
		Summary:         fmt.Sprintf("Redacted note #%d", noteIndex),
		Ref:             fmt.Sprintf("note:%d", noteIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("note:%d", noteIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) RedactCREvent(id, eventIndex int, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("redaction reason cannot be empty")
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return err
	}
	if guardErr := s.ensureNoMergeInProgressForCR(cr); guardErr != nil {
		return guardErr
	}

	idx, err := oneBasedIndex(eventIndex, len(cr.Events), "event")
	if err != nil {
		return err
	}
	if cr.Events[idx].Redacted || cr.Events[idx].Summary == redactedPlaceholder {
		return ErrAlreadyRedacted
	}

	cr.Events[idx].Summary = redactedPlaceholder
	cr.Events[idx].Redacted = true
	cr.Events[idx].RedactionReason = reason
	if cr.Events[idx].Meta == nil {
		cr.Events[idx].Meta = map[string]string{}
	}
	cr.Events[idx].Meta["redacted_via"] = fmt.Sprintf("event:%d", eventIndex)

	now := s.timestamp()
	actor := s.git.Actor()
	cr.UpdatedAt = now
	cr.Events = append(cr.Events, model.Event{
		TS:              now,
		Actor:           actor,
		Type:            model.EventTypeCRRedacted,
		Summary:         fmt.Sprintf("Redacted event #%d", eventIndex),
		Ref:             fmt.Sprintf("event:%d", eventIndex),
		RedactionReason: reason,
		Meta: map[string]string{
			"target": fmt.Sprintf("event:%d", eventIndex),
			"reason": reason,
		},
	})
	return s.store.SaveCR(cr)
}

func (s *Service) HistoryCR(id int, showRedacted bool) (*CRHistory, error) {
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}

	history := &CRHistory{
		CRID:        cr.ID,
		Title:       cr.Title,
		Status:      cr.Status,
		Description: cr.Description,
		Notes:       make([]HistoryNote, 0, len(cr.Notes)),
		Evidence:    make([]HistoryEvidence, 0, len(cr.Evidence)),
		Events:      make([]HistoryEvent, 0, len(cr.Events)),
	}

	for i, note := range cr.Notes {
		redacted := note == redactedPlaceholder
		text := note
		if redacted {
			text = redactedPlaceholder
		}
		history.Notes = append(history.Notes, HistoryNote{
			Index:    i + 1,
			Text:     text,
			Redacted: redacted,
		})
	}

	for i, entry := range cr.Evidence {
		attachments := append([]string(nil), entry.Attachments...)
		var exitCode *int
		if entry.ExitCode != nil {
			value := *entry.ExitCode
			exitCode = &value
		}
		history.Evidence = append(history.Evidence, HistoryEvidence{
			Index:       i + 1,
			TS:          entry.TS,
			Actor:       entry.Actor,
			Type:        entry.Type,
			Scope:       entry.Scope,
			Command:     entry.Command,
			ExitCode:    exitCode,
			OutputHash:  entry.OutputHash,
			Summary:     entry.Summary,
			Attachments: attachments,
		})
	}

	for i, event := range cr.Events {
		summary := event.Summary
		redacted := event.Redacted || summary == redactedPlaceholder
		if redacted {
			summary = redactedPlaceholder
		}
		reason := ""
		if showRedacted {
			reason = event.RedactionReason
		}
		meta := map[string]string(nil)
		if showRedacted && len(event.Meta) > 0 {
			meta = cloneStringMap(event.Meta)
		}
		history.Events = append(history.Events, HistoryEvent{
			Index:           i + 1,
			TS:              event.TS,
			Actor:           event.Actor,
			Type:            event.Type,
			Summary:         summary,
			Ref:             event.Ref,
			Redacted:        redacted,
			RedactionReason: reason,
			Meta:            meta,
		})
	}

	return history, nil
}
