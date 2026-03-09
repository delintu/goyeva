import os
import sys

from dataclasses import dataclass


@dataclass
class Flag:
    name: str
    arg: str | None = None

    def __str__(self):
        return self.name + (" " + self.arg if self.arg else "")


TEMP_NAME = "__temp_build.exe"
FLAGS = [Flag("-std=c99"), Flag("-D_CRT_SECURE_NO_WARNINGS")]


def main():
    command = f"clang -o {TEMP_NAME}"

    flags = map(lambda x: str(x), FLAGS)
    command += " " + " ".join(flags)

    files = []
    for d, _, f in os.walk("."):
        dfls = filter(lambda x: x.endswith(".c"), f)
        dfls = map(lambda x: os.path.join(d, x), dfls)
        files.extend(dfls)
    command += " " + " ".join(files)

    os.system(command)

    try:
        call_command = (
            ".\\"
            + TEMP_NAME
            + (" " + " ".join(sys.argv[1:]) if len(sys.argv) > 1 else "")
        )
        if wait_status := os.system(call_command):
            exit_code = os.waitstatus_to_exitcode(wait_status)
            print(f"error: exit code: {exit_code}")
    finally:
        os.unlink(TEMP_NAME)


main()
