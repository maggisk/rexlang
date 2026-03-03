-- Number literals: hex, octal, binary, underscore separators

-- Hex literals
let hex1 = 0xFF        -- 255
let hex2 = 0x1A3F      -- 6719

-- Octal literals
let oct1 = 0o77        -- 63
let oct2 = 0o755       -- 493

-- Binary literals
let bin1 = 0b1010      -- 10
let bin2 = 0b11111111  -- 255

-- Underscore separators
let million = 1_000_000
let hexColor = 0xFF_00_FF
let binByte = 0b1111_0000
let piApprox = 3.141_592

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
