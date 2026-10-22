#!/usr/bin/env python3
"""Drive a real mosaid-cli session and assert the observable behaviour.

The driver writes lines to the CLI's stdin and reads whole replies from its
stdout. When the CLI prints an approval prompt it extracts the token and
approves with it, exactly as the owner would in Telegram.
"""
import os
import re
import select
import subprocess
import sys
import time

PROMPT = re.compile(r"/approve (\S+)")

# (request, text the reply must contain)
EXPECTATIONS = [
    ("أعطني محتويات مساحة العمل", "محتوى مساحة العمل"),
    ("اكتب notes/todo.md", "موافقة مطلوبة"),
    ("اقرأ notes/todo.md", "مراجعة التقرير"),
]

IDLE = 0.35
DEADLINE = 40.0


def read_turn(process, timeout=DEADLINE):
    """Read one printed block: accumulate bytes until the CLI goes quiet.

    Reads the raw file descriptor so buffered text I/O cannot hide a line from
    select() and split one reply into two.
    """
    deadline = time.time() + timeout
    fd = process.stdout.fileno()
    buffer = b""
    while time.time() < deadline:
        ready, _, _ = select.select([fd], [], [], IDLE)
        if not ready:
            if buffer or process.poll() is not None:
                break
            continue
        chunk = os.read(fd, 65536)
        if not chunk:
            break
        buffer += chunk
    return buffer.decode("utf-8", "replace")


def send(process, line):
    process.stdin.write((line + "\n").encode("utf-8"))
    process.stdin.flush()


def main():
    command = sys.argv[1:]
    if not command:
        raise SystemExit("usage: demo_driver.py <command...>")
    process = subprocess.Popen(
        command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, bufsize=0,
    )
    transcript = []
    try:
        banner = read_turn(process)
        if "Type /help" not in banner:
            raise SystemExit("CLI did not start:\n" + banner)
        transcript.append(banner.strip())
        for request, expected in EXPECTATIONS:
            send(process, request)
            output = read_turn(process)
            transcript.append("▸ " + request + "\n" + output.strip())
            if expected not in output:
                raise SystemExit("expected %r in reply:\n%s" % (expected, output))
            match = PROMPT.search(output)
            if match:
                send(process, "/approve " + match.group(1))
                approved = read_turn(process)
                transcript.append("▸ /approve <token>\n" + approved.strip())
                if "✅" not in approved and "تم" not in approved:
                    raise SystemExit("approval was not executed:\n%s" % approved)
        send(process, "/exit")
        process.wait(timeout=20)
    finally:
        if process.poll() is None:
            process.kill()
    print("\n\n".join(transcript))
    print("\nDRIVER: all expectations met")


if __name__ == "__main__":
    main()
