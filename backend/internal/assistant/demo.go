package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/menzurka228/psb-legal-assistant/backend/internal/kb"
	"github.com/menzurka228/psb-legal-assistant/backend/internal/llm"
)

// Demo answers by returning excerpts from a local keyword search.
type Demo struct {
	retriever *kb.Retriever
	chunks    []kb.Chunk
}

func NewDemo(base *kb.KnowledgeBase) *Demo {
	chunks := make([]kb.Chunk, 0, len(base.CompanyChunks)+len(base.LawChunks))
	chunks = append(chunks, base.CompanyChunks...)
	chunks = append(chunks, base.LawChunks...)
	return &Demo{retriever: kb.NewRetriever(chunks, nil), chunks: chunks}
}

func (d *Demo) Answer(ctx context.Context, question string, history []llm.Message) (Answer, error) {
	if answer, ok := d.example(question); ok {
		return answer, nil
	}

	var previousQuestion string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			previousQuestion = history[i].Content
			break
		}
	}

	hits := d.retriever.Search(ctx, question, previousQuestion, 12, 2)
	if len(hits) == 0 {
		return Answer{
			Answer:  "Деморежим. По этому запросу ничего не нашлось. Уточните название документа или номер статьи.",
			Sources: []Source{},
			Refused: true,
		}, nil
	}

	var answer strings.Builder
	answer.WriteString("Деморежим. По запросу нашлись такие выдержки из базы:\n")
	sources := make([]Source, 0, len(hits))
	for i, hit := range hits {
		n := i + 1
		text := []rune(strings.Join(strings.Fields(hit.Text), " "))
		excerpt := string(text)
		if len(text) > 420 {
			excerpt = string(text[:420]) + "…"
		}
		fmt.Fprintf(&answer, "\n**%s** [%d]\n\n%s\n", hit.DocTitle, n, excerpt)
		sources = append(sources, Source{N: n, Hit: hit})
	}
	return Answer{Answer: answer.String(), Sources: sources}, nil
}

type demoEvidence struct {
	document string
	text     string
}

var demoExamples = map[string]struct {
	answer   string
	evidence []demoEvidence
}{
	"Какой гарантийный срок по договору с «Альфа-Трейд»?": {
		"24 месяца с даты поставки по п. 5.2 договора поставки с ООО «Альфа-Трейд» [1].",
		[]demoEvidence{{"Альфа-Трейд", "Гарантийный срок на Товар составляет 24"}},
	},
	"Договор на 12 млн рублей заключали в августе 2024 года. Нужно ли было одобрение генерального директора?": {
		"Да. В августе 2024 года действовала редакция регламента от 10.01.2023. Пункт 2.4 требовал одобрения генерального директора до подписания договора на сумму свыше 10 млн рублей [1].",
		[]demoEvidence{{"ред 2023", "свыше 10 000 000"}},
	},
	"Какой МРОТ был в 2024 году?": {
		"В базе есть редакция статьи 1, которая устанавливает МРОТ с 1 января 2026 года. Размер за 2024 год по этому документу подтвердить нельзя [1].",
		[]demoEvidence{{"МРОТ", "27 093"}},
	},
	"Как согласовать договор поставки с иностранной компанией на 5 млн рублей?": {
		"По общему порядку согласуйте проект с юротделом, главным бухгалтером и финансовым директором, поскольку сумма превышает 3 млн рублей [1]. До подписания пройдите юридическую проверку и валютный контроль [2], а также получите заключение службы комплаенс по итогам санкционной проверки [3].",
		[]demoEvidence{{"ред 2025", "свыше 3 000 000"}, {"ред 2025", "Договоры с иностранными контрагентами до подписания"}, {"К-12", "санкционную проверку контрагента"}},
	},
}

func (d *Demo) example(question string) (Answer, bool) {
	example, ok := demoExamples[question]
	if !ok {
		return Answer{}, false
	}
	sources := make([]Source, 0, len(example.evidence))
	for _, evidence := range example.evidence {
		found := false
		for _, chunk := range d.chunks {
			if strings.Contains(chunk.Doc, evidence.document) && strings.Contains(chunk.Text, evidence.text) {
				sources = append(sources, Source{N: len(sources) + 1, Hit: kb.Hit{Chunk: chunk}})
				found = true
				break
			}
		}
		if !found {
			return Answer{}, false
		}
	}
	return Answer{Answer: "Деморежим. " + example.answer, Sources: sources}, true
}
