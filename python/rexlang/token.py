from dataclasses import dataclass, field
from typing import Any


@dataclass
class Token:
    # kind is a keyword/symbol string (e.g. 'let', '+', '->', ';;')
    # or one of: 'int', 'string', 'bool', 'ident', 'eof'
    kind: str
    value: Any = None
    line: int = field(default=1, compare=False)
    col: int = field(default=0, compare=False)

    def __str__(self):
        if self.value is not None:
            return f"{self.kind}({self.value})"
        return self.kind
