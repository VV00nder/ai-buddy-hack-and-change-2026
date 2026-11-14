package app

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/knowledge"
)

// Инструкции, добавленные в интерфейсе: руководитель и сотрудники с правом
// PermKB пишут инструкции для своей команды и раскладывают их по папкам.

// KBDoc — инструкция, сохранённая в хранилище.
type KBDoc struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Folder    string   `json:"folder"`
	Audience  []string `json:"audience"`
	Team      string   `json:"team"`
	Tags      []string `json:"tags"`
	Body      string   `json:"body"`
	UpdatedBy string   `json:"updatedBy"`
	UpdatedAt string   `json:"updatedAt"`
	// Source — "answer", если инструкция оформлена из ответа на вопрос стажёра.
	Source string `json:"source,omitempty"`
}

// DocInput — поля инструкции из редактора.
type DocInput struct {
	Title    string   `json:"title"`
	Folder   string   `json:"folder"`
	Audience []string `json:"audience"`
	Tags     []string `json:"tags"`
	Body     string   `json:"body"`
}

// kbHidden — пометка об удалённой инструкции.
type kbHidden struct {
	ID string `json:"id"`
	By string `json:"by"`
	At string `json:"at"`
}

// KBFolder — пустая папка, созданная заранее.
type KBFolder struct {
	Path string `json:"path"`
	Team string `json:"team"`
}

func (d KBDoc) toDoc() *knowledge.Doc {
	return &knowledge.Doc{ID: d.ID, Title: d.Title, Folder: d.Folder, Audience: d.Audience, Team: d.Team, Tags: d.Tags,
		Body: d.Body, Custom: true, UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt, Source: d.Source}
}

// LoadKB поднимает инструкции, добавленные в интерфейсе.
func (a *App) LoadKB(ctx context.Context) error {
	docs, err := listRec[KBDoc](ctx, a.store, recKBDoc)
	if err != nil {
		return err
	}
	for _, d := range docs {
		a.kb.Upsert(d.toDoc())
	}
	// удалённые инструкции (в том числе встроенные) не возвращаются после перезапуска
	hidden, err := listRec[kbHidden](ctx, a.store, recKBHidden)
	if err != nil {
		return err
	}
	for _, h := range hidden {
		a.kb.Delete(h.ID)
	}
	return nil
}

// CanEditDoc — можно ли пользователю править и удалять инструкцию: администратор — любую,
// руководитель — своего отдела и общие, сотрудник с правом на инструкции — своего отдела.
func (a *App) CanEditDoc(ctx context.Context, u domain.User, d *knowledge.Doc) bool {
	if !a.canEditKB(ctx, u) {
		return false
	}
	return isAdmin(u) || d.Team == u.Team || (d.Team == "all" && u.Role == domain.RoleLead)
}

var validAudience = []string{"all", string(domain.RoleIntern), string(domain.RoleMentor), string(domain.RoleLead), string(domain.RoleHRBP), string(domain.RoleEngineer)}

func cleanDocInput(in DocInput) (DocInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(strings.ReplaceAll(in.Body, "\r\n", "\n"))
	in.Folder = knowledge.CleanFolder(in.Folder)
	switch {
	case utf8.RuneCountInString(in.Title) < 3 || utf8.RuneCountInString(in.Title) > 150:
		return in, invalid("Название — от 3 до 150 символов")
	case utf8.RuneCountInString(in.Body) < 10 || utf8.RuneCountInString(in.Body) > 20000:
		return in, invalid("Текст инструкции — от 10 до 20 000 символов")
	}
	if err := checkFolder(in.Folder); err != nil {
		return in, err
	}
	var aud []string
	for _, r := range in.Audience {
		if !slices.Contains(validAudience, r) {
			return in, invalid("Неизвестная роль «%s»", r)
		}
		if !slices.Contains(aud, r) {
			aud = append(aud, r)
		}
	}
	if len(aud) == 0 || slices.Contains(aud, "all") {
		aud = []string{"all"}
	}
	in.Audience = aud
	var tags []string
	for _, t := range in.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && utf8.RuneCountInString(t) <= 40 && !slices.Contains(tags, t) && len(tags) < 20 {
			tags = append(tags, t)
		}
	}
	in.Tags = tags
	return in, nil
}

func checkFolder(p string) error {
	if utf8.RuneCountInString(p) > 120 {
		return invalid("Путь папки длиннее 120 символов")
	}
	if strings.Count(p, "/") > 3 {
		return invalid("Папки вкладываются не глубже 4 уровней")
	}
	return nil
}

// SaveDoc создаёт (id пустой) или изменяет инструкцию команды.
func (a *App) SaveDoc(ctx context.Context, u domain.User, id string, in DocInput) (*knowledge.Doc, error) {
	if !a.canEditKB(ctx, u) {
		return nil, ErrForbidden
	}
	in, err := cleanDocInput(in)
	if err != nil {
		return nil, err
	}
	team, source := contentTeam(u), ""
	if id == "" {
		id = newID("doc")
	} else {
		cur, ok := a.kb.Raw(id)
		switch {
		case !ok:
			return nil, ErrNotFound
		case !a.CanEditDoc(ctx, u, cur):
			return nil, ErrForbidden
		}
		team, source = cur.Team, cur.Source // правка не переносит инструкцию в другой отдел
	}
	rec := KBDoc{ID: id, Title: in.Title, Folder: in.Folder, Audience: in.Audience, Team: team, Tags: in.Tags, Body: in.Body,
		UpdatedBy: u.Name, UpdatedAt: a.now().Format(time.RFC3339), Source: source}
	if err := putRec(ctx, a.store, recKBDoc, id, rec); err != nil {
		return nil, err
	}
	d := rec.toDoc()
	a.kb.Upsert(d)
	a.Track(ctx, u.ID, "kb_doc_saved", map[string]any{"doc": id})
	return d, nil
}

// DeleteDoc удаляет инструкцию команды.
func (a *App) DeleteDoc(ctx context.Context, u domain.User, id string) error {
	cur, ok := a.kb.Raw(id)
	if !ok {
		return ErrNotFound
	}
	if !a.CanEditDoc(ctx, u, cur) {
		return ErrForbidden
	}
	if err := a.store.DeleteRecord(ctx, recKBDoc, id); err != nil {
		return err
	}
	// пометка нужна встроенным инструкциям и ответам наставников, чтобы они не вернулись при запуске
	if err := putRec(ctx, a.store, recKBHidden, id, kbHidden{ID: id, By: u.ID, At: a.now().Format(time.RFC3339)}); err != nil {
		return err
	}
	a.kb.Delete(id)
	a.Track(ctx, u.ID, "kb_doc_deleted", map[string]any{"doc": id})
	return nil
}

// Folders — папки, видимые пользователю: из доступных инструкций и созданные командой.
func (a *App) Folders(ctx context.Context, u domain.User) ([]string, error) {
	set := map[string]bool{}
	add := func(p string) {
		parts := strings.Split(p, "/")
		for i := range parts {
			set[strings.Join(parts[:i+1], "/")] = true
		}
	}
	for _, d := range a.kb.List(u) {
		if d.Folder != "" {
			add(d.Folder)
		}
	}
	fs, err := listRec[KBFolder](ctx, a.store, recKBFolder)
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		if f.Team == u.Team || f.Team == "all" || isAdmin(u) {
			add(f.Path)
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// CreateFolder создаёт папку (в том числе вложенную: «Процессы/Релизы»).
func (a *App) CreateFolder(ctx context.Context, u domain.User, path string) ([]string, error) {
	if !a.canEditKB(ctx, u) {
		return nil, ErrForbidden
	}
	path = knowledge.CleanFolder(path)
	if path == "" {
		return nil, invalid("Укажите название папки")
	}
	if err := checkFolder(path); err != nil {
		return nil, err
	}
	if err := putRec(ctx, a.store, recKBFolder, contentTeam(u)+"|"+path, KBFolder{Path: path, Team: contentTeam(u)}); err != nil {
		return nil, err
	}
	return a.Folders(ctx, u)
}

// DeleteFolder удаляет пустую папку команды.
func (a *App) DeleteFolder(ctx context.Context, u domain.User, path string) ([]string, error) {
	if !a.canEditKB(ctx, u) {
		return nil, ErrForbidden
	}
	path = knowledge.CleanFolder(path)
	for _, d := range a.kb.List(u) {
		if d.Folder == path || strings.HasPrefix(d.Folder, path+"/") {
			return nil, invalid("В папке есть инструкции — сначала перенесите их")
		}
	}
	fs, err := listRec[KBFolder](ctx, a.store, recKBFolder)
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		if sameTeam(u, f.Team) && (f.Path == path || strings.HasPrefix(f.Path, path+"/")) {
			if err := a.store.DeleteRecord(ctx, recKBFolder, f.Team+"|"+f.Path); err != nil {
				return nil, err
			}
		}
	}
	return a.Folders(ctx, u)
}
