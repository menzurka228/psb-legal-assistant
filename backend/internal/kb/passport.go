package kb

// Паспорта документов компании и хронология изменений законов — извлекаются автоматически из текстов
// (без ручной разметки), чтобы модель видела даты, редакции, стороны и связи между документами.
// Порт build_passport / link_passports / law_timeline из submission/solution.py.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var months = map[string]time.Month{
	"января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6,
	"июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12,
}

var (
	dateRe     = regexp.MustCompile(`(?i)(\d{1,2})\s+(января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\s+(\d{4})|(\d{2})\.(\d{2})\.(\d{4})`)
	numRe      = regexp.MustCompile(`№\s*([А-ЯЁA-Z0-9][\pL\pN_\-/]*)`)
	partyRe    = regexp.MustCompile(`((?:ООО|АО|ПАО|ЗАО|ИП)\s+«[^»]+»)\s*\((?:ИНН\s*(\d+)\)[^«]{0,90}«([^»]+)»|«([^»]+)»\))`)
	approvedRe = regexp.MustCompile(`Утвержд\pL* приказом[^№\n]*№\s*([\pL\pN\-/]+)\s+от\s+(\S+)`)
	redRe      = regexp.MustCompile(`Редакция от\s+(\S+)`)
	refContrRe = regexp.MustCompile(`(?i)к договору[^\n]*`)
	termOpenRe = regexp.MustCompile(`(?i)неопредел[её]нный срок`)
	validToRe  = regexp.MustCompile(`(?i)действует до\s+([^.,;\n]+)`)
	autoRe     = regexp.MustCompile(`(?i)автоматически продлева`)
	lostRe     = regexp.MustCompile(`(?i)утрат\pL* силу с\s+(\S+)`)
	fromRe     = regexp.MustCompile(`Действует с\s+(\S+)`)
	amendsRe   = regexp.MustCompile(`(?i)к договору[^№\n]*№\s*([\pL\pN\-/]+)`)
	setPointRe = regexp.MustCompile(`[Ии]зложить пункт\pL*\s+([\d.]+)`)
	applyRe    = regexp.MustCompile(`(?i)применя\pL+ к|вступает в силу`)
	sinceRe    = regexp.MustCompile(`^\s*С\s+\d{1,2}\s+\pL+\s+\d{4}`)
	refRegRe   = regexp.MustCompile(`п\.\s*[\d.]+\s+Регламента[^)\n]*\)?`)
	docTypes   = []struct {
		re   *regexp.Regexp
		name string
	}{
		{regexp.MustCompile(`(?i)ДОПОЛНИТЕЛЬНОЕ СОГЛАШЕНИЕ`), "дополнительное соглашение"},
		{regexp.MustCompile(`(?m)^\s*ДОГОВОР\s+([А-ЯЁ ]+?)(?:\s+№|$)`), "договор"},
		{regexp.MustCompile(`(?m)^\s*РЕГЛАМЕНТ\s+([А-ЯЁ ]+?)(?:\s+ООО|$)`), "регламент"},
		{regexp.MustCompile(`(?i)СЛУЖЕБНАЯ ЗАПИСКА`), "служебная записка"},
		{regexp.MustCompile(`(?i)Исх\.\s*№`), "письмо"},
	}
)

// ParseDate находит первую дату в тексте: «1 августа 2024» или «01.08.2024».
func ParseDate(s string) (time.Time, bool) {
	m := dateRe.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false
	}
	if m[1] != "" {
		d, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[3])
		return time.Date(y, months[strings.ToLower(m[2])], d, 0, 0, 0, 0, time.UTC), true
	}
	d, _ := strconv.Atoi(m[4])
	mo, _ := strconv.Atoi(m[5])
	y, _ := strconv.Atoi(m[6])
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC), true
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("02.01.2006")
}

func normNum(n string) string {
	return strings.Trim(strings.ReplaceAll(strings.ToUpper(n), "/", "-"), ".,;")
}

type Passport struct {
	Doc       *Document
	DocType   string
	Subject   string
	Number    string
	Date      time.Time
	Parties   []string
	Term      string
	ValidFrom time.Time
	ValidTo   time.Time
	LostForce time.Time
	Amends    string
	Changes   []string
	Notes     []string
}

func BuildPassport(d *Document) *Passport {
	lines := strings.Split(d.Text, "\n")
	header := strings.Join(lines[:min(8, len(lines))], "\n")
	p := &Passport{Doc: d}
	for _, t := range docTypes {
		if m := t.re.FindStringSubmatch(header); m != nil {
			p.DocType = t.name
			if len(m) > 1 {
				p.Subject = strings.ToLower(strings.TrimSpace(m[1]))
			}
			break
		}
	}
	if p.DocType == "регламент" {
		if m := approvedRe.FindStringSubmatch(header); m != nil {
			p.Notes = append(p.Notes, fmt.Sprintf("утверждён приказом № %s от %s", m[1], strings.TrimRight(m[2], ".")))
		}
	} else if m := numRe.FindStringSubmatch(header); m != nil {
		p.Number = strings.Trim(m[1], ".,")
	}
	if m := redRe.FindStringSubmatch(header); m != nil {
		p.Date, _ = ParseDate(m[1])
	}
	if p.Date.IsZero() {
		p.Date, _ = ParseDate(refContrRe.ReplaceAllString(header, ""))
	}
	head := d.Text
	if r := []rune(head); len(r) > 1500 {
		head = string(r[:1500])
	}
	for _, m := range partyRe.FindAllStringSubmatch(head, -1) {
		entry := m[1]
		if m[2] != "" {
			entry += ", ИНН " + m[2]
		}
		if role := m[3] + m[4]; role != "" {
			entry += " — " + role
		}
		if !contains(p.Parties, entry) {
			p.Parties = append(p.Parties, entry)
		}
	}
	if termOpenRe.MatchString(d.Text) {
		p.Term = "на неопределённый срок"
	}
	if m := validToRe.FindStringSubmatch(d.Text); m != nil {
		if t, ok := ParseDate(m[1]); ok {
			p.ValidTo = t
			p.Term = "до " + fmtDate(t)
			if autoRe.MatchString(d.Text) {
				p.Term += ", с автоматическим продлением на каждый следующий год, если ни одна сторона не заявит о прекращении"
			}
		}
	}
	if m := lostRe.FindStringSubmatch(d.Text); m != nil {
		p.LostForce, _ = ParseDate(m[1])
	}
	if m := fromRe.FindStringSubmatch(d.Text); m != nil {
		p.ValidFrom, _ = ParseDate(m[1])
	}
	if m := amendsRe.FindStringSubmatch(d.Text); m != nil {
		p.Amends = m[1]
		for _, pt := range setPointRe.FindAllStringSubmatch(d.Text, -1) {
			p.Changes = append(p.Changes, "новая редакция п. "+strings.TrimRight(pt[1], "."))
		}
		for _, l := range lines {
			if applyRe.MatchString(l) {
				p.Notes = append(p.Notes, strings.TrimSpace(l))
			}
		}
	}
	for _, l := range lines {
		if sinceRe.MatchString(l) {
			p.Notes = append(p.Notes, Truncate(strings.TrimSpace(l), 220))
		}
		for _, ref := range refRegRe.FindAllString(l, -1) {
			p.Notes = append(p.Notes, "ссылается на "+strings.TrimSpace(ref))
		}
	}
	return p
}

// LinkPassports связывает редакции одного документа и доп. соглашения с договорами.
func LinkPassports(ps []*Passport) {
	groups := map[string][]*Passport{}
	for _, p := range ps {
		if p.DocType == "регламент" || (!p.LostForce.IsZero() && p.DocType != "") {
			k := p.DocType + "|" + p.Subject
			groups[k] = append(groups[k], p)
		}
	}
	start := func(p *Passport) time.Time {
		if !p.ValidFrom.IsZero() {
			return p.ValidFrom
		}
		return p.Date
	}
	for _, eds := range groups {
		sort.Slice(eds, func(i, j int) bool { return start(eds[i]).Before(start(eds[j])) })
		for i := 0; i+1 < len(eds); i++ {
			prev, next := eds[i], eds[i+1]
			if s := start(next); !s.IsZero() {
				if prev.ValidTo.IsZero() {
					prev.ValidTo = s.AddDate(0, 0, -1)
				}
				if prev.LostForce.IsZero() {
					prev.LostForce = s
				}
			}
		}
		for _, e := range eds {
			if e.ValidFrom.IsZero() {
				e.ValidFrom = e.Date
			}
		}
	}
	byNum := map[string]*Passport{}
	for _, p := range ps {
		if p.Number != "" && p.DocType == "договор" {
			byNum[normNum(p.Number)] = p
		}
	}
	for _, p := range ps {
		if base, ok := byNum[normNum(p.Amends)]; ok && p.Amends != "" {
			ch := strings.Join(p.Changes, "; ")
			if ch == "" {
				ch = "см. текст"
			}
			base.Notes = append(base.Notes, fmt.Sprintf("изменён: %s (%s)", p.Doc.Title, ch))
		}
	}
}

// Status — статус документа на дату.
func (p *Passport) Status(today time.Time) string {
	if !p.LostForce.IsZero() && !p.LostForce.After(today) {
		return "УТРАТИЛ СИЛУ с " + fmtDate(p.LostForce)
	}
	if p.DocType == "договор" && !p.ValidTo.IsZero() && p.ValidTo.Before(today) {
		if strings.Contains(p.Term, "продлен") {
			return "действует за счёт автопродления, если ни одна сторона не заявила о прекращении (сведений о заявлениях в базе нет)"
		}
		return "срок истёк " + fmtDate(p.ValidTo)
	}
	return "действует"
}

func (p *Passport) Format(today time.Time) string {
	kind := "документ"
	if p.DocType != "" {
		kind = strings.TrimSpace(p.DocType + " " + p.Subject)
	}
	head := "• " + p.Doc.Title + ": " + kind
	if p.Number != "" {
		head += " № " + p.Number
	}
	if !p.Date.IsZero() {
		head += " от " + fmtDate(p.Date)
	}
	out := []string{head}
	if len(p.Parties) > 0 {
		out = append(out, "  стороны: "+strings.Join(p.Parties, "; "))
	}
	if p.DocType == "регламент" || !p.LostForce.IsZero() {
		to := "настоящее время"
		if !p.ValidTo.IsZero() {
			to = fmtDate(p.ValidTo)
		}
		out = append(out, fmt.Sprintf("  период действия редакции: с %s по %s", fmtDate(p.ValidFrom), to))
	}
	if p.Term != "" {
		out = append(out, "  срок действия: "+p.Term)
	}
	if p.Amends != "" {
		ch := strings.Join(p.Changes, "; ")
		if ch == "" {
			ch = "см. текст"
		}
		out = append(out, fmt.Sprintf("  изменяет договор № %s: %s", p.Amends, ch))
	}
	seen := map[string]bool{}
	for _, n := range p.Notes {
		if !seen[n] {
			seen[n] = true
			out = append(out, "  "+n)
		}
	}
	out = append(out, fmt.Sprintf("  статус на %s: %s", fmtDate(today), p.Status(today)))
	return strings.Join(out, "\n")
}

// Passports строит и форматирует паспорта всех документов компании.
func (kb *KnowledgeBase) Passports(today time.Time) string {
	var ps []*Passport
	for _, d := range kb.Company {
		ps = append(ps, BuildPassport(d))
	}
	LinkPassports(ps)
	var out []string
	for _, p := range ps {
		out = append(out, p.Format(today))
	}
	return strings.Join(out, "\n")
}

var (
	changeRe      = regexp.MustCompile(`(?i)^(.{0,120}?(?:изменен\pL*|дополнен\pL*|утратил\pL*\s+силу|исключен\pL*|применяются).{0,160}?)\s+с\s+(\d{1,2}\s+\pL+\s+\d{4})\s*г`)
	fzTailRe      = regexp.MustCompile(`\s+-\s+Федеральный закон.*$`)
	fzHeadRe      = regexp.MustCompile(`^Федеральным законом от .*?N\s*\d+-ФЗ\s*`)
	timelineSince = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
)

// LawTimeline собирает пометки об изменениях закона (с 2020 г.) с отметкой «в будущем».
func LawTimeline(d *Document, today time.Time) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(d.Text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || runeLen(line) > 400 {
			continue
		}
		m := changeRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		when, ok := ParseDate(m[2])
		if !ok || when.Before(timelineSince) {
			continue
		}
		what := fzHeadRe.ReplaceAllString(strings.TrimSpace(fzTailRe.ReplaceAllString(m[1], "")), "")
		mark := "действует"
		if when.After(today) {
			mark = "вступит в силу в будущем"
		}
		e := fmt.Sprintf("%s — с %s (%s)", what, fmtDate(when), mark)
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
