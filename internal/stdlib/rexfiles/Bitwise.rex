export bitAnd, bitOr, bitXor, bitNot, shiftLeft, shiftRight


test "bitAnd" =
    assert bitAnd 0xFF 0x0F == 0x0F
    assert bitAnd 0b1100 0b1010 == 0b1000

test "bitOr" =
    assert bitOr 0xFF 0x0F == 0xFF
    assert bitOr 0b1100 0b1010 == 0b1110

test "bitXor" =
    assert bitXor 0xFF 0x0F == 0xF0
    assert bitXor 0b1100 0b1010 == 0b0110

test "bitNot" =
    -- bitNot inverts all bits; bitAnd to mask to 8 bits for readability
    assert bitAnd (bitNot 0x0F) 0xFF == 0xF0

test "shiftLeft" =
    assert shiftLeft 1 4 == 16
    assert shiftLeft 0b0001 3 == 0b1000

test "shiftRight" =
    assert shiftRight 16 4 == 1
    assert shiftRight 0b1000 3 == 0b0001
