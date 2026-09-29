"""Run synthetic installer interactions with a controlling terminal and piped source."""

import json
import os
import pty
import re
import select
import signal
import sys
import termios
import time


def main():
    scenario = json.load(sys.stdin)
    source_read, source_write = os.pipe()
    done_read, done_write = os.pipe()
    pid, master = pty.fork()
    if pid == 0:
        os.close(source_write)
        os.close(done_write)
        os.dup2(source_read, 0)
        os.dup2(done_read, 4)
        os.set_inheritable(4, True)
        os.close(source_read)
        if done_read != 4:
            os.close(done_read)
        # Inspect terminal settings before its owner exits and macOS revokes the terminal.
        keeper = '''trap ':' INT TERM
printf '__INSTALL_TEST_READY__\\n'
"$@"
result=$?
printf '\\n__INSTALL_TEST_EXIT_%s__\\n' "$result"
IFS= read -r acknowledgement <&4
exit "$result"
'''
        os.execvp("bash", [
            "bash", "-c", keeper, "pty-host", scenario.get("shell", "bash"),
            "-s", "--", *scenario.get("args", []),
        ])

    os.close(source_read)
    os.close(done_read)
    output = bytearray()
    status = None

    def poll():
        nonlocal status
        if status is None:
            finished, result = os.waitpid(pid, os.WNOHANG)
            if finished:
                status = os.waitstatus_to_exitcode(result)
        return status

    def read_output(timeout=0.05):
        if select.select([master], [], [], timeout)[0]:
            try:
                chunk = os.read(master, 65536)
            except OSError:
                return False
            if not chunk:
                return False
            output.extend(chunk)
        return True

    def completed():
        return re.search(rb"__INSTALL_TEST_EXIT_(-?\d+)__", output)

    error = None
    restored = False
    changes = []
    try:
        deadline = time.monotonic() + 8
        while b"__INSTALL_TEST_READY__" not in output:
            read_output()
            if time.monotonic() > deadline:
                raise TimeoutError("Terminal owner did not start")
        initial = termios.tcgetattr(master)
        with os.fdopen(source_write, "wb") as source:
            source.write(scenario["source"].encode())
        offset = 0
        for step in scenario.get("steps", []):
            while step["wait"].encode() not in output[offset:]:
                read_output()
                if completed() or poll() is not None:
                    break
                if time.monotonic() > deadline:
                    raise TimeoutError(f"Waiting for {step['wait']!r}")
            if completed() or poll() is not None:
                break
            # Wait for Bash's silent key read so control keys are not sent during menu rendering.
            while step.get("raw") and termios.tcgetattr(master)[3] & termios.ECHO:
                read_output(0.01)
                if time.monotonic() > deadline:
                    raise TimeoutError("Selector did not start reading keys")
            offset = len(output)
            os.write(master, step["send"].encode())
        while not completed() and poll() is None:
            read_output()
            if time.monotonic() > deadline:
                raise TimeoutError("Installer did not finish")
        final = termios.tcgetattr(master)
        # Darwin marks pending input for retyping after canonical mode is restored.
        initial[3] &= ~getattr(termios, "PENDIN", 0)
        final[3] &= ~getattr(termios, "PENDIN", 0)
        restored = final == initial
        changes = [
            (index, repr(before), repr(after))
            for index, (before, after) in enumerate(zip(initial, final))
            if before != after
        ]
        os.write(done_write, b"\n")
        while poll() is None:
            read_output()
            if time.monotonic() > deadline:
                raise TimeoutError("Terminal owner did not exit")
    except (BrokenPipeError, TimeoutError) as failure:
        error = str(failure)
    finally:
        if poll() is None:
            os.kill(pid, signal.SIGKILL)
            _, result = os.waitpid(pid, 0)
            status = os.waitstatus_to_exitcode(result)
        os.close(master)
        os.close(done_write)
    print(json.dumps({
        "status": status,
        "output": output.decode(errors="replace"),
        "restored": restored,
        "changes": changes,
        "error": error,
    }))


if __name__ == "__main__":
    main()
