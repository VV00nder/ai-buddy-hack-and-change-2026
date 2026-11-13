package corp

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"aibuddy/internal/domain"
	"aibuddy/internal/identity"
	"aibuddy/internal/mcp"
)

// Регистрация сотрудников и перевод между отделами. В Авито это делает
// кадровая система и справочник; здесь — их имитация с тем же поведением:
// у сотрудника всегда есть руководитель отдела, у стажёра — наставник.

type cred struct {
	Salt []byte `json:"salt"`
	Hash []byte `json:"hash"`
}

const pbkdf2Iter = 100_000

func hashPassword(pw string, salt []byte) []byte {
	h, _ := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	return h
}

// RegisterInput — данные регистрации.
type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Team     string `json:"team"`
	Role     string `json:"role"` // intern | engineer
}

// RegisterError — ошибка проверки данных с текстом для пользователя.
type RegisterError struct{ Msg string }

func (e *RegisterError) Error() string { return e.Msg }

func regErr(f string, a ...any) error { return &RegisterError{Msg: fmt.Sprintf(f, a...)} }

// TeamInfo — отдел и его руководитель (для выбора при регистрации и переводе).
type TeamInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	LeadID   string `json:"leadId"`
	LeadName string `json:"leadName"`
}

// leadOf — руководитель отдела. Вызывается под s.mu.
func (s *Service) leadOf(team string) *Person {
	for _, id := range s.sortedIDs() {
		if p := s.users[id]; p.Team == team && p.Role == domain.RoleLead {
			return p
		}
	}
	return nil
}

// mentorOf — наставник отдела для нового стажёра (первый наставник, иначе руководитель).
func (s *Service) mentorOf(team string) *Person {
	for _, id := range s.sortedIDs() {
		if p := s.users[id]; p.Team == team && p.Role == domain.RoleMentor {
			return p
		}
	}
	return s.leadOf(team)
}

func (s *Service) sortedIDs() []string {
	ids := make([]string, 0, len(s.users))
	for id := range s.users {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Teams — отделы, в которых есть руководитель.
func (s *Service) Teams() []TeamInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.teamsLocked()
}

func (s *Service) teamsLocked() []TeamInfo {
	out := []TeamInfo{}
	for _, t := range s.teams {
		if l := s.leadOf(t.ID); l != nil {
			out = append(out, TeamInfo{ID: t.ID, Name: t.Name, LeadID: l.ID, LeadName: l.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func validPassword(pw string) bool {
	if utf8.RuneCountInString(pw) < 8 || len(pw) > 128 {
		return false
	}
	var letter, digit bool
	for _, r := range pw {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	return letter && digit
}

// Register создаёт учётную запись сотрудника в выбранном отделе.
func (s *Service) Register(in RegisterInput) (domain.User, error) {
	name := strings.Join(strings.Fields(in.Name), " ")
	email := strings.ToLower(strings.TrimSpace(in.Email))
	switch {
	case len(strings.Fields(name)) < 2 || utf8.RuneCountInString(name) > 100:
		return domain.User{}, regErr("Укажите фамилию и имя (и отчество, если есть)")
	case !strings.Contains(email, "@") || len(email) > 120:
		return domain.User{}, regErr("Укажите корректную почту")
	case !validPassword(in.Password):
		return domain.User{}, regErr("Пароль — не короче 8 символов, с буквами и цифрами")
	case in.Role != string(domain.RoleIntern) && in.Role != string(domain.RoleEngineer):
		return domain.User{}, regErr("Выберите роль: стажёр или сотрудник")
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return domain.User{}, regErr("Укажите корректную почту")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return domain.User{}, errOutage
	}
	if _, ok := s.byLogin[email]; ok {
		return domain.User{}, regErr("Эта почта уже зарегистрирована — войдите")
	}
	team, ok := s.teams[in.Team]
	lead := s.leadOf(in.Team)
	if !ok || lead == nil {
		return domain.User{}, regErr("Выберите отдел из списка")
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	p := &Person{User: domain.User{ID: s.nextID("u"), Login: email, Email: email, Name: name, Role: domain.Role(in.Role),
		Team: team.ID, TeamName: team.Name, LeadID: lead.ID, StartDate: s.today(), Registered: true}}
	p.Handle = "@" + strings.SplitN(email, "@", 2)[0]
	if p.Role == domain.RoleIntern {
		if m := s.mentorOf(team.ID); m != nil {
			p.MentorID = m.ID
		}
	}
	s.users[p.ID] = p
	s.byLogin[email] = p
	s.order = append(s.order, email)
	s.creds[p.ID] = cred{Salt: salt, Hash: hashPassword(in.Password, salt)}
	s.balance[p.ID] = 3
	s.persist()
	return p.User, nil
}

// CheckPassword проверяет пароль: у зарегистрированных — свой, у тестовых — общий демо-пароль.
func (s *Service) CheckPassword(login, password, demoPassword string) (domain.User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byLogin[strings.ToLower(strings.TrimSpace(login))]
	if !ok {
		return domain.User{}, false
	}
	if c, ok := s.creds[p.ID]; ok {
		return p.User, hmac.Equal(hashPassword(password, c.Salt), c.Hash)
	}
	return p.User, !p.Registered && password == demoPassword
}

// Transfer — руководитель переводит сотрудника своего отдела в другой отдел.
// Руководитель нового отдела становится начальником сотрудника, стажёр получает
// наставника нового отдела; заявки, которые ждали решения прежнего руководителя,
// переходят новому.
func (s *Service) Transfer(c identity.Claims, userID, teamID string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outage {
		return nil, errOutage
	}
	lead, ok := s.users[c.Sub]
	if !ok || lead.Role != domain.RoleLead {
		return nil, mcp.ErrForbidden
	}
	p, ok := s.users[userID]
	switch {
	case !ok:
		return nil, mcp.NewToolError("not_found", "Сотрудник не найден")
	case p.ID == lead.ID:
		return nil, mcp.NewToolError("self", "Нельзя перевести самого себя")
	case p.Team != lead.Team:
		return nil, mcp.NewToolError("other_team", "Переводить можно только сотрудников своего отдела")
	case p.Role == domain.RoleLead:
		return nil, mcp.NewToolError("not_allowed_for_role", "Руководителя отдела переводит кадровая служба")
	case teamID == p.Team:
		return nil, mcp.NewToolError("same_team", "Сотрудник уже работает в этом отделе")
	}
	team, ok := s.teams[teamID]
	newLead := s.leadOf(teamID)
	if !ok || newLead == nil {
		return nil, mcp.NewToolError("bad_team", "Выберите отдел из списка")
	}
	oldLead := p.LeadID
	p.Team, p.TeamName, p.LeadID = team.ID, team.Name, newLead.ID
	if p.Role == domain.RoleIntern {
		p.MentorID = ""
		if m := s.mentorOf(team.ID); m != nil {
			p.MentorID = m.ID
		}
	} else {
		p.MentorID = ""
	}
	delete(s.deputies[oldLead], p.ID) // заместителем прежнего руководителя он больше не является
	moved := 0
	for _, a := range s.absences {
		if a.UserID == p.ID && a.Status == "pending" && a.ApproverID == oldLead {
			a.ApproverID, a.Approver = newLead.ID, newLead.Name
			moved++
		}
	}
	for _, r := range s.access {
		if r.UserID == p.ID && r.Status == "pending" && r.ApproverID == oldLead {
			r.ApproverID, r.Approver = newLead.ID, newLead.Name
			moved++
		}
	}
	s.persist()
	out := map[string]any{"user": p.User, "lead": map[string]any{"id": newLead.ID, "name": newLead.Name}, "movedRequests": moved}
	if m := s.users[p.MentorID]; m != nil {
		out["mentor"] = map[string]any{"id": m.ID, "name": m.Name}
	}
	return out, nil
}

// refresh — актуальные отдел, роль, руководитель и наставник из справочника:
// после перевода старый токен не даёт прав в прежнем отделе.
func (s *Service) refresh(c identity.Claims) identity.Claims {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.users[c.Sub]; ok {
		c.Role, c.Team, c.LeadID, c.MentorID = p.Role, p.Team, p.LeadID, p.MentorID
	}
	return c
}

// --- Сохранение регистраций и переводов между перезапусками ---

type savedState struct {
	Users []Person        `json:"users"`
	Creds map[string]cred `json:"creds"`
	Seq   int             `json:"seq"`
}

// UseStateFile включает сохранение регистраций и переводов в файл и загружает его.
func (s *Service) UseStateFile(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateFile = path
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var st savedState
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("corp state: %w", err)
	}
	for i := range st.Users {
		saved := st.Users[i]
		if cur, ok := s.users[saved.ID]; ok { // тестовый сотрудник: применяем перевод
			cur.Team, cur.TeamName, cur.LeadID, cur.MentorID = saved.Team, saved.TeamName, saved.LeadID, saved.MentorID
			continue
		}
		p := saved
		s.users[p.ID] = &p
		s.byLogin[p.Login] = &p
		s.order = append(s.order, p.Login)
		if _, ok := s.balance[p.ID]; !ok {
			s.balance[p.ID] = 3
		}
	}
	for id, c := range st.Creds {
		s.creds[id] = c
	}
	s.seq = max(s.seq, st.Seq)
	return nil
}

// persist сохраняет состояние справочника. Вызывается под s.mu.
func (s *Service) persist() {
	if s.stateFile == "" {
		return
	}
	st := savedState{Creds: s.creds, Seq: s.seq}
	for _, id := range s.sortedIDs() {
		st.Users = append(st.Users, *s.users[id])
	}
	b, _ := json.Marshal(st)
	_ = os.MkdirAll(filepath.Dir(s.stateFile), 0o755)
	tmp := s.stateFile + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, s.stateFile)
	}
}
