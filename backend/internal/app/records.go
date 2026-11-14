package app

import (
	"context"
	"encoding/json"
	"errors"

	"aibuddy/internal/store"
)

// Виды записей в общем хранилище.
const (
	recKBDoc       = "kb_doc"
	recKBFolder    = "kb_folder"
	recKBHidden    = "kb_hidden"
	recQuiz        = "quiz"
	recQuizAttempt = "quiz_attempt"
	recTemplate    = "template"
	recProgram     = "program"
	recGrant       = "grant"
	recQuizLock    = "quiz_lock"
	recReminder    = "reminder"
)

func putRec[T any](ctx context.Context, st store.Store, kind, id string, v T) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return st.PutRecord(ctx, kind, id, b)
}

func getRec[T any](ctx context.Context, st store.Store, kind, id string) (T, error) {
	var v T
	b, err := st.Record(ctx, kind, id)
	if errors.Is(err, store.ErrNotFound) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func listRec[T any](ctx context.Context, st store.Store, kind string) ([]T, error) {
	raw, err := st.Records(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raw))
	for _, b := range raw {
		var v T
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
