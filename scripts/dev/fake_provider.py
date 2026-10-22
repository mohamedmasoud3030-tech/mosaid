#!/usr/bin/env python3
"""A scripted OpenAI-compatible provider for offline Mosaid demos.

It is deliberately dumb: it reads the last message, decides which tool the
"model" wants, and answers once the tool observation arrives. It exists so the
real binary, real policy engine, real approval engine and real workspace tools
can be exercised without a paid key or a Telegram bot.

Usage: fake_provider.py <cert.pem> <key.pem> <port>
"""
import json
import ssl
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

STATE = {"forced_429": False, "requests": 0}
LOCK = threading.Lock()


def tool_call(call_id, name, arguments):
    return {
        "id": call_id,
        "type": "function",
        "function": {"name": name, "arguments": json.dumps(arguments, ensure_ascii=False)},
    }


def observation(messages):
    for message in reversed(messages):
        if message.get("role") == "tool":
            try:
                return json.loads(message.get("content") or "{}")
            except json.JSONDecodeError:
                return {"ok": False, "error": "unparseable observation"}
    return None


def respond(messages):
    last = messages[-1] if messages else {}
    if last.get("role") == "tool":
        result = observation(messages) or {}
        if not result.get("ok"):
            text = "تعذّر تنفيذ ذلك: " + str(result.get("error", "خطأ غير معروف"))[:200]
        else:
            text = "تم ✅ " + summary(result.get("tool"), result.get("result"))
        return {"content": text}

    text = (last.get("content") or "").lower()
    if "اكتب" in text or "write" in text or "أنشئ" in text:
        return {"tool_calls": [tool_call("call_write", "workspace.write", {
            "path": "notes/todo.md",
            "content": "- مراجعة التقرير\n- اجتماع 10:00\n- رد على البريد\n",
        })]}
    if "اقرأ" in text or "read" in text:
        return {"tool_calls": [tool_call("call_read", "workspace.read", {"path": "notes/todo.md"})]}
    if "ابحث" in text or "search" in text:
        return {"tool_calls": [tool_call("call_search", "workspace.search", {"query": "اجتماع", "path": "."})]}
    return {"tool_calls": [tool_call("call_list", "workspace.list", {"path": "."})]}


def summary(tool, value):
    if tool == "workspace.read" and isinstance(value, str):
        return "قرأت الملف، أول سطرين:\n" + "\n".join(value.strip().splitlines()[:2])
    if tool == "workspace.list" and isinstance(value, list):
        names = ", ".join(item.get("name", "?") for item in value[:10]) or "(فارغ)"
        return "محتوى مساحة العمل: " + names
    if tool == "workspace.search" and isinstance(value, list):
        return "نتائج البحث: " + ("; ".join(value[:3]) if value else "لا نتائج")
    if isinstance(value, dict):
        return json.dumps(value, ensure_ascii=False)[:300]
    return str(value)[:300]


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *args):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        with LOCK:
            STATE["requests"] += 1
            force = not STATE["forced_429"]
            STATE["forced_429"] = True
        if force:
            # Prove the client retries transient rate limits instead of failing.
            body = json.dumps({"error": {"message": "rate limited (demo)", "type": "rate_limit"}}).encode()
            self.send_response(429)
            self.send_header("Retry-After", "1")
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        messages = payload.get("messages", [])
        tools = payload.get("tools", [])
        reply = respond(messages)
        message = {"role": "assistant", "content": reply.get("content")}
        if reply.get("tool_calls"):
            if not tools:
                message = {"role": "assistant", "content": "لا توجد أدوات متاحة في هذه الجلسة."}
            else:
                message["content"] = None
                message["tool_calls"] = reply["tool_calls"]
        body = json.dumps({
            "model": payload.get("model", "fake"),
            "choices": [{
                "message": message,
                "finish_reason": "tool_calls" if reply.get("tool_calls") else "stop",
            }],
            "usage": {"prompt_tokens": 120, "completion_tokens": 40, "total_tokens": 160},
        }, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main():
    cert, key, port = sys.argv[1], sys.argv[2], int(sys.argv[3])
    server = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(cert, key)
    server.socket = context.wrap_socket(server.socket, server_side=True)
    print("ready", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
