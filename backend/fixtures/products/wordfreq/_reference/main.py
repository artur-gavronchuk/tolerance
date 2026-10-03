import re
import sys
from collections import Counter


def main(argv):
    top = 10
    args = argv[1:]
    if args:
        if len(args) != 2 or args[0] != "--top" or not args[1].isdigit() or int(args[1]) < 1:
            print("usage: main.py [--top N]", file=sys.stderr)
            return 2
        top = int(args[1])
    words = re.findall(r"[a-z0-9']+", sys.stdin.read().lower())
    for word, n in sorted(Counter(words).items(), key=lambda kv: (-kv[1], kv[0]))[:top]:
        print(word, n)
    return 0


sys.exit(main(sys.argv))
