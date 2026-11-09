"""Веб-сервер: API чата, просмотр документов и статический интерфейс."""
from pathlib import Path

from fastapi import FastAPI, HTTPException
from fastapi.responses import FileResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from .assistant import LegalAssistant

app = FastAPI(title="Юридический ассистент ПСБ")
assistant = LegalAssistant()
STATIC = Path(__file__).parent / "static"


class Message(BaseModel):
    role: str
    content: str


class ChatRequest(BaseModel):
    question: str
    history: list[Message] = []


@app.get("/api/health")
def health():
    return {"status": "ok", "documents": len(assistant.kb.documents), "chunks": len(assistant.kb.chunks)}


@app.post("/api/chat")
def chat(req: ChatRequest):
    return assistant.answer(req.question, [m.model_dump() for m in req.history])


@app.get("/api/documents")
def documents():
    return sorted(assistant.kb.documents)


@app.get("/api/documents/{name:path}")
def document(name: str):
    if name not in assistant.kb.documents:
        raise HTTPException(404, "Документ не найден")
    return {"name": name, "text": assistant.kb.documents[name]}


app.mount("/static", StaticFiles(directory=STATIC), name="static")


@app.get("/")
def index():
    return FileResponse(STATIC / "index.html")
