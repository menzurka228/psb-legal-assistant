package kb

import "testing"

func TestSearchFindsArticle(t *testing.T) {
	base := FromDocuments(map[string]string{
		"law.txt": "Статья 1. Общие положения\nНастоящий закон регулирует кредит.\n" +
			"Статья 2. Ставка НДС\nСтавка налога на добавленную стоимость составляет 20 процентов.\n",
	})
	hits := base.Search("какая ставка НДС", 5)
	if len(hits) == 0 || hits[0].Title != "Статья 2. Ставка НДС" {
		t.Fatalf("ожидали Статью 2 первой, получили %+v", hits)
	}
}

func TestPointTitleIncludesArticle(t *testing.T) {
	chunks := SplitIntoChunks("nk.txt", "Статья 168. Сумма налога\n1. При реализации товаров налог предъявляется покупателю.")
	if got := chunks[len(chunks)-1].Title; got != "Статья 168. Сумма налога › п. 1" {
		t.Fatalf("неверный заголовок пункта: %q", got)
	}
}

func TestSearchNoMatch(t *testing.T) {
	base := FromDocuments(map[string]string{"a.txt": "Статья 1. Договор займа"})
	if hits := base.Search("рецепт борща", 5); len(hits) != 0 {
		t.Fatalf("не ожидали совпадений, получили %+v", hits)
	}
}

func TestSplitLongChunk(t *testing.T) {
	long := make([]rune, maxChunkLen*2+10)
	for i := range long {
		long[i] = 'а'
	}
	if got := len(SplitIntoChunks("d", string(long))); got != 3 {
		t.Fatalf("ожидали 3 фрагмента, получили %d", got)
	}
}
