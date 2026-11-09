from legal_assistant.kb import KnowledgeBase


def test_search_finds_article(tmp_path):
    (tmp_path / "law.txt").write_text(
        "Статья 1. Общие положения\nНастоящий закон регулирует кредит.\n"
        "Статья 2. Ставка НДС\nСтавка налога на добавленную стоимость составляет 20 процентов.\n"
        "Статья 3. Порядок согласования\nДоговор согласуется с юридическим департаментом.\n",
        encoding="utf-8",
    )
    kb = KnowledgeBase(tmp_path)
    hits = kb.search("какая ставка НДС")
    assert hits and hits[0][0].title.startswith("Статья 2")
