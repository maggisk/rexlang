-- Number literals: hex, octal, binary, underscore separators

-- Hex literals
hex1 = 0xFF        -- 255
hex2 = 0x1A3F      -- 6719

-- Octal literals
oct1 = 0o77        -- 63
oct2 = 0o755       -- 493

-- Binary literals
bin1 = 0b1010      -- 10
bin2 = 0b11111111  -- 255

-- Underscore separators
million = 1_000_000
hexColor = 0xFF_00_FF
binByte = 0b1111_0000
piApprox = 3.141_592

test "hex literals" =
    assert (hex1 == 255)
    assert (hex2 == 6719)

test "octal literals" =
    assert (oct1 == 63)
    assert (oct2 == 493)

test "binary literals" =
    assert (bin1 == 10)
    assert (bin2 == 255)

test "underscore separators" =
    assert (million == 1000000)
    assert (hexColor == 0xFF00FF)
    assert (binByte == 0b11110000)
