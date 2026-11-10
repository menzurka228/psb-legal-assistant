package kb

import (
	"strings"
	"testing"
	"time"
)

// Синтетические документы — не из базы кейса: проверяем, что извлечение не подогнано под наши файлы.
const contract = `ДОГОВОР ПОДРЯДА № Д-3/2025
г. Казань — 5 марта 2025 г.
ООО «Ромашка» (ИНН 1650000001), именуемое в дальнейшем «Заказчик», в лице директора, и
ООО «Лютик» (ИНН 1650000002), именуемое в дальнейшем «Подрядчик», заключили договор.
9.1. Договор вступает в силу с даты подписания и действует до 31 декабря 2025 г.`
const amendment = `ДОПОЛНИТЕЛЬНОЕ СОГЛАШЕНИЕ № 2
к договору подряда № Д-3/2025 от 5 марта 2025 г.
г. Казань — 10 июня 2025 г.
Изложить пункт 4.1 договора в следующей редакции: «…».
Новая редакция применяется к работам, начатым после 1 июля 2025 г.`
const oldEd = "РЕГЛАМЕНТ ЗАКУПОК ООО «РОМАШКА»\nУтверждён приказом № 1-П от 01.02.2022\nРедакция от 01.02.2022\n1. Общие положения"
const newEd = "РЕГЛАМЕНТ ЗАКУПОК ООО «РОМАШКА»\nУтверждён приказом № 9-П от 15.05.2025\nРедакция от 01.06.2025. Действует с 01.06.2025.\n1. Общие положения"

func day(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }

func passports() map[string]*Passport {
	var ps []*Passport
	out := map[string]*Passport{}
	for name, text := range map[string]string{"Договор Д-3": contract, "ДС 2": amendment, "Регламент 2022": oldEd, "Регламент 2025": newEd} {
		p := BuildPassport(MakeDocument("документы компании__"+name+".txt", text))
		ps = append(ps, p)
		out[name] = p
	}
	LinkPassports(ps)
	return out
}

func TestPassportContract(t *testing.T) {
	p := passports()["Договор Д-3"]
	if p.DocType != "договор" || p.Number != "Д-3/2025" || !p.Date.Equal(day(2025, 3, 5)) || !p.ValidTo.Equal(day(2025, 12, 31)) {
		t.Fatalf("паспорт договора: %+v", p)
	}
	if !strings.Contains(strings.Join(p.Parties, ";"), "ИНН 1650000002 — Подрядчик") || !strings.HasPrefix(p.Status(day(2026, 1, 10)), "срок истёк") {
		t.Fatalf("стороны/статус: %+v %s", p.Parties, p.Status(day(2026, 1, 10)))
	}
}

func TestAmendmentLinked(t *testing.T) {
	ps := passports()
	ds, c := ps["ДС 2"], ps["Договор Д-3"]
	if ds.Amends != "Д-3/2025" || !ds.Date.Equal(day(2025, 6, 10)) || !strings.Contains(strings.Join(c.Notes, ";"), "ДС 2 (новая редакция п. 4.1)") {
		t.Fatalf("связь доп. соглашения: %+v / %+v", ds, c.Notes)
	}
}

func TestEditions(t *testing.T) {
	ps := passports()
	old, cur := ps["Регламент 2022"], ps["Регламент 2025"]
	if !old.ValidTo.Equal(day(2025, 5, 31)) || !old.LostForce.Equal(day(2025, 6, 1)) || old.Number != "" {
		t.Fatalf("старая редакция: %+v", old)
	}
	if !cur.ValidFrom.Equal(day(2025, 6, 1)) || cur.Status(day(2026, 1, 1)) != "действует" || !strings.HasPrefix(old.Status(day(2026, 1, 1)), "УТРАТИЛ СИЛУ") {
		t.Fatalf("статусы редакций: %+v", cur)
	}
}

func TestLawTimeline(t *testing.T) {
	d := &Document{Text: "Пункт 2 изменен с 1 января 2026 г. - Федеральный закон от 1 мая 2025 г. N 1-ФЗ\nПункт 3 изменен с 1 марта 2027 г. - Федеральный закон\nПункт 4 изменен с 1 января 2010 г."}
	got := LawTimeline(d, day(2026, 11, 9))
	want := []string{"Пункт 2 изменен — с 01.01.2026 (действует)", "Пункт 3 изменен — с 01.03.2027 (вступит в силу в будущем)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("хронология: %q", got)
	}
}
