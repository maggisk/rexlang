import Std:Result (Ok, Err)
import Std:Maybe (Just, Nothing)
import Std:String (padLeft, substring, parseInt, length)


-- # Types

-- | Absolute point in time (epoch milliseconds).
export opaque type Instant = | Instant Int

-- | A signed duration in milliseconds.
export opaque type Duration = | Duration Int

-- | Wall-clock date and time components (no timezone).
export type DateTimeParts = {
    year : Int,
    month : Int,
    day : Int,
    hour : Int,
    minute : Int,
    second : Int,
    millisecond : Int
}

-- | Day of the week.
export type Weekday
    = Monday
    | Tuesday
    | Wednesday
    | Thursday
    | Friday
    | Saturday
    | Sunday


-- # Calendar math (internal)

-- | Whether a year is a leap year.
isLeapYear : Int -> Bool
isLeapYear y =
    (y % 4 == 0 && y % 100 != 0) || y % 400 == 0

test "isLeapYear" =
    assert isLeapYear 2000
    assert isLeapYear 2024
    assert not (isLeapYear 1900)
    assert not (isLeapYear 2023)

-- | Days in a given month (1-indexed).
daysInMonth : Int -> Int -> Int
daysInMonth y m =
    match m
        when 1 ->
            31
        when 2 ->
            if isLeapYear y then
                29
            else
                28
        when 3 ->
            31
        when 4 ->
            30
        when 5 ->
            31
        when 6 ->
            30
        when 7 ->
            31
        when 8 ->
            31
        when 9 ->
            30
        when 10 ->
            31
        when 11 ->
            30
        when _ ->
            31

test "daysInMonth" =
    assert daysInMonth 2024 2 == 29
    assert daysInMonth 2023 2 == 28
    assert daysInMonth 2024 1 == 31
    assert daysInMonth 2024 4 == 30


-- | Convert epoch milliseconds + UTC offset (minutes) to date/time parts.
-- Uses the civil time algorithm from Howard Hinnant (public domain).
millisToParts : Int -> Int -> DateTimeParts
millisToParts epochMs offsetMin =
    let
        ms = epochMs + offsetMin * 60000
        -- Split into days and time-of-day
        totalSec =
            if ms >= 0 then
                ms / 1000
            else
                (ms - 999) / 1000
        daysSinceEpoch =
            if totalSec >= 0 then
                totalSec / 86400
            else
                (totalSec - 86399) / 86400
        timeOfDay = ms - daysSinceEpoch * 86400000
        -- Shift epoch from 1970-01-01 to 0000-03-01
        z = daysSinceEpoch + 719468
        -- Era (400-year cycle)
        era =
            if z >= 0 then
                z / 146097
            else
                (z - 146096) / 146097
        doe = z - era * 146097
        yoeRaw = (doe - doe / 1461 + doe / 36524 - doe / 146096) / 365
        -- Clamp to [0, 399]: the formula gives 400 at the exact era boundary (doe=146096)
        yoe =
            if yoeRaw > 399 then
                399
            else
                yoeRaw
        y = yoe + era * 400
        doy = doe - (365 * yoe + yoe / 4 - yoe / 100)
        mp = (5 * doy + 2) / 153
        d = doy - (153 * mp + 2) / 5 + 1
        m =
            if mp < 10 then
                mp + 3
            else
                mp - 9
        year =
            if m <= 2 then
                y + 1
            else
                y
        hour = timeOfDay / 3600000
        minute = (timeOfDay % 3600000) / 60000
        second = (timeOfDay % 60000) / 1000
        millisecond = timeOfDay % 1000
    in
    DateTimeParts {
        year = year,
        month = m,
        day = d,
        hour = hour,
        minute = minute,
        second = second,
        millisecond = millisecond
    }

test "millisToParts epoch" =
    let p = millisToParts 0 0
    assert p.year == 1970
    assert p.month == 1
    assert p.day == 1
    assert p.hour == 0

test "millisToParts known date" =
    -- 2024-06-15T12:30:45.123Z — compute via partsToMillis for accuracy
    let expected = DateTimeParts { year = 2024, month = 6, day = 15, hour = 12, minute = 30, second = 45, millisecond = 123 }
    let ms = partsToMillis expected 0
    let p = millisToParts ms 0
    assert p.year == 2024
    assert p.month == 6
    assert p.day == 15
    assert p.hour == 12
    assert p.minute == 30
    assert p.second == 45
    assert p.millisecond == 123

test "millisToParts with offset" =
    -- Epoch + 5:30 offset = 1970-01-01 05:30:00
    let p = millisToParts 0 330
    assert p.year == 1970
    assert p.month == 1
    assert p.day == 1
    assert p.hour == 5
    assert p.minute == 30

test "millisToParts leap day" =
    -- 2000-02-29T00:00:00Z = 951782400000 ms
    let p = millisToParts 951782400000 0
    assert p.year == 2000
    assert p.month == 2
    assert p.day == 29


-- | Convert date/time parts + UTC offset (minutes) to epoch milliseconds.
partsToMillis : DateTimeParts -> Int -> Int
partsToMillis parts offsetMin =
    let
        -- Shift March-based year
        y =
            if parts.month <= 2 then
                parts.year - 1
            else
                parts.year
        m =
            if parts.month > 2 then
                parts.month - 3
            else
                parts.month + 9
        era =
            if y >= 0 then
                y / 400
            else
                (y - 399) / 400
        yoe = y - era * 400
        doy = (153 * m + 2) / 5 + parts.day - 1
        doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
        daysSinceEpoch = era * 146097 + doe - 719468
        timeMs = parts.hour * 3600000 + parts.minute * 60000 + parts.second * 1000 + parts.millisecond
    in
    daysSinceEpoch * 86400000 + timeMs - offsetMin * 60000

test "partsToMillis epoch" =
    let p = DateTimeParts { year = 1970, month = 1, day = 1, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert partsToMillis p 0 == 0

test "partsToMillis round-trip" =
    let ms = 1718451045123
    let p = millisToParts ms 0
    assert partsToMillis p 0 == ms

test "partsToMillis with offset" =
    let p = DateTimeParts { year = 1970, month = 1, day = 1, hour = 5, minute = 30, second = 0, millisecond = 0 }
    assert partsToMillis p 330 == 0

test "partsToMillis leap day" =
    let p = DateTimeParts { year = 2000, month = 2, day = 29, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert partsToMillis p 0 == 951782400000


-- # Formatting (internal)

-- | Pad an integer to n digits with leading zeros.
padNum : Int -> Int -> String
padNum width n =
    let
        raw =
            if n < 0 then
                "${0 - n}"
            else
                "${n}"
    in
    padLeft width "0" raw

-- | Replace format tokens in a pattern with values from DateTimeParts.
formatParts : String -> DateTimeParts -> String
formatParts fmt parts =
    let rec go i acc =
        if i >= length fmt then
            acc
        else
            let remaining = length fmt - i
            in
            if remaining >= 4 && substring i (i + 4) fmt == "YYYY" then
                go (i + 4) (acc ++ padNum 4 parts.year)
            else if remaining >= 3 && substring i (i + 3) fmt == "SSS" then
                go (i + 3) (acc ++ padNum 3 parts.millisecond)
            else if remaining >= 2 && substring i (i + 2) fmt == "MM" then
                go (i + 2) (acc ++ padNum 2 parts.month)
            else if remaining >= 2 && substring i (i + 2) fmt == "DD" then
                go (i + 2) (acc ++ padNum 2 parts.day)
            else if remaining >= 2 && substring i (i + 2) fmt == "HH" then
                go (i + 2) (acc ++ padNum 2 parts.hour)
            else if remaining >= 2 && substring i (i + 2) fmt == "mm" then
                go (i + 2) (acc ++ padNum 2 parts.minute)
            else if remaining >= 2 && substring i (i + 2) fmt == "ss" then
                go (i + 2) (acc ++ padNum 2 parts.second)
            else
                go (i + 1) (acc ++ substring i (i + 1) fmt)
    in go 0 ""

test "formatParts ISO" =
    let p = DateTimeParts { year = 2024, month = 6, day = 15, hour = 12, minute = 30, second = 45, millisecond = 123 }
    assert formatParts "YYYY-MM-DDTHH:mm:ss.SSS" p == "2024-06-15T12:30:45.123"

test "formatParts date only" =
    let p = DateTimeParts { year = 1970, month = 1, day = 1, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert formatParts "YYYY-MM-DD" p == "1970-01-01"


-- # Parsing (internal)

-- | Try to extract n digits from input at position i.
readNum : Int -> Int -> String -> Result (Int, Int) String
readNum i width input =
    if i + width > length input then
        Err "input too short"
    else
        let chunk = substring i (i + width) input
        in
        match parseInt chunk
            when Just n ->
                Ok (n, i + width)
            when Nothing ->
                Err ("expected number, got " ++ chunk)

-- | Parse a date/time string using a format pattern.
-- Returns Ok DateTimeParts or Err message.
parseParts : String -> String -> Result DateTimeParts String
parseParts fmt input =
    let rec go fi ii yr mo dy hr mi sc ms =
        if fi >= length fmt then
            if ii >= length input then
                Ok (DateTimeParts { year = yr, month = mo, day = dy, hour = hr, minute = mi, second = sc, millisecond = ms })
            else
                Err "input has extra characters"
        else
            let remaining = length fmt - fi
            in
            if remaining >= 4 && substring fi (fi + 4) fmt == "YYYY" then
                match readNum ii 4 input
                    when Ok (n, ii2) ->
                        go (fi + 4) ii2 n mo dy hr mi sc ms
                    when Err e ->
                        Err e
            else if remaining >= 3 && substring fi (fi + 3) fmt == "SSS" then
                match readNum ii 3 input
                    when Ok (n, ii2) ->
                        go (fi + 3) ii2 yr mo dy hr mi sc n
                    when Err e ->
                        Err e
            else if remaining >= 2 && substring fi (fi + 2) fmt == "MM" then
                match readNum ii 2 input
                    when Ok (n, ii2) ->
                        go (fi + 2) ii2 yr n dy hr mi sc ms
                    when Err e ->
                        Err e
            else if remaining >= 2 && substring fi (fi + 2) fmt == "DD" then
                match readNum ii 2 input
                    when Ok (n, ii2) ->
                        go (fi + 2) ii2 yr mo n hr mi sc ms
                    when Err e ->
                        Err e
            else if remaining >= 2 && substring fi (fi + 2) fmt == "HH" then
                match readNum ii 2 input
                    when Ok (n, ii2) ->
                        go (fi + 2) ii2 yr mo dy n mi sc ms
                    when Err e ->
                        Err e
            else if remaining >= 2 && substring fi (fi + 2) fmt == "mm" then
                match readNum ii 2 input
                    when Ok (n, ii2) ->
                        go (fi + 2) ii2 yr mo dy hr n sc ms
                    when Err e ->
                        Err e
            else if remaining >= 2 && substring fi (fi + 2) fmt == "ss" then
                match readNum ii 2 input
                    when Ok (n, ii2) ->
                        go (fi + 2) ii2 yr mo dy hr mi n ms
                    when Err e ->
                        Err e
            else
                -- Literal character: must match
                if ii >= length input then
                    Err "input too short"
                else if substring fi (fi + 1) fmt == substring ii (ii + 1) input then
                    go (fi + 1) (ii + 1) yr mo dy hr mi sc ms
                else
                    Err "unexpected character"
    in
    go 0 0 0 1 1 0 0 0 0

test "parseParts ISO" =
    let p =
        match parseParts "YYYY-MM-DDTHH:mm:ss" "2024-06-15T12:30:45"
            when Ok p ->
                p
            when Err _ ->
                DateTimeParts { year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert p.year == 2024
    assert p.month == 6
    assert p.day == 15
    assert p.hour == 12
    assert p.minute == 30
    assert p.second == 45

test "parseParts date only" =
    let p =
        match parseParts "YYYY-MM-DD" "2024-06-15"
            when Ok p ->
                p
            when Err _ ->
                DateTimeParts { year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert p.year == 2024
    assert p.month == 6

test "parseParts error on bad input" =
    let isErr =
        match parseParts "YYYY-MM-DD" "not-a-date"
            when Ok _ ->
                false
            when Err _ ->
                true
    assert isErr


-- # Instant creation

-- | Current time.
export
now : () -> Instant
now _ = Instant (dateTimeNow ())


-- | Create an Instant from epoch milliseconds.
export
fromMillis : Int -> Instant
fromMillis ms = Instant ms

test "fromMillis round-trip" =
    let ms = 1700000000000
    let i = fromMillis ms
    assert toMillis i == ms


-- | Create an Instant from date/time parts (interpreted as UTC).
export
fromParts : DateTimeParts -> Instant
fromParts parts = Instant (partsToMillis parts 0)

test "fromParts epoch" =
    let parts = DateTimeParts { year = 1970, month = 1, day = 1, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert toMillis (fromParts parts) == 0

test "fromParts known date" =
    let parts = DateTimeParts { year = 2024, month = 6, day = 15, hour = 12, minute = 30, second = 45, millisecond = 0 }
    let i = fromParts parts
    let back = toParts i
    assert back.year == 2024
    assert back.month == 6
    assert back.day == 15
    assert back.hour == 12
    assert back.minute == 30
    assert back.second == 45


-- | Create an Instant from date/time parts (interpreted as local time).
export
fromLocalParts : DateTimeParts -> Instant
fromLocalParts parts = Instant (partsToMillis parts (dateTimeUtcOffset ()))


-- | Parse a date/time string using a format pattern.
-- Supported tokens: YYYY, MM, DD, HH, mm, ss, SSS.
export
parse : String -> String -> Result Instant String
parse fmt input =
    match parseParts fmt input
        when Ok parts ->
            Ok (Instant (partsToMillis parts 0))
        when Err e ->
            Err e

test "parse ISO date" =
    let result = parse "YYYY-MM-DDTHH:mm:ss" "2024-06-15T12:30:45"
    let p =
        match result
            when Ok i ->
                toParts i
            when Err _ ->
                DateTimeParts { year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0, millisecond = 0 }
    assert p.year == 2024
    assert p.month == 6
    assert p.day == 15


-- # Instant accessors

-- | Extract epoch milliseconds from an Instant.
export
toMillis : Instant -> Int
toMillis instant =
    match instant
        when Instant ms ->
            ms


-- | Decompose an Instant into date/time parts (UTC).
export
toParts : Instant -> DateTimeParts
toParts instant =
    match instant
        when Instant ms ->
            millisToParts ms 0

test "toParts epoch" =
    let p = toParts (fromMillis 0)
    assert p.year == 1970
    assert p.month == 1
    assert p.day == 1
    assert p.hour == 0
    assert p.minute == 0
    assert p.second == 0
    assert p.millisecond == 0


-- | Decompose an Instant into date/time parts (local timezone).
export
toLocalParts : Instant -> DateTimeParts
toLocalParts instant =
    match instant
        when Instant ms ->
            millisToParts ms (dateTimeUtcOffset ())


-- | Format an Instant using a format pattern (UTC).
-- Supported tokens: YYYY, MM, DD, HH, mm, ss, SSS.
export
format : String -> Instant -> String
format fmt instant =
    match instant
        when Instant ms ->
            formatParts fmt (millisToParts ms 0)

test "format ISO" =
    let i = fromMillis 0
    assert format "YYYY-MM-DD" i == "1970-01-01"
    assert format "HH:mm:ss" i == "00:00:00"

test "format known date" =
    let result = parse "YYYY-MM-DDTHH:mm:ss" "2024-06-15T12:30:45"
    let formatted =
        match result
            when Ok i ->
                format "YYYY-MM-DDTHH:mm:ss" i
            when Err _ ->
                ""
    assert formatted == "2024-06-15T12:30:45"


-- | Format an Instant using a format pattern (local timezone).
export
formatLocal : String -> Instant -> String
formatLocal fmt instant =
    match instant
        when Instant ms ->
            formatParts fmt (millisToParts ms (dateTimeUtcOffset ()))


-- # Weekday

-- | Get the day of the week for an Instant (UTC).
export
weekday : Instant -> Weekday
weekday instant =
    match instant
        when Instant ms ->
            let
                -- Handle negative millis (dates before epoch) correctly
                epochDays =
                    if ms >= 0 then
                        ms / 86400000
                    else
                        (ms - 86399999) / 86400000
                -- Jan 1 1970 was Thursday; Monday = 0
                dow = ((epochDays + 3) % 7 + 7) % 7
            in
            match dow
                when 0 ->
                    Monday
                when 1 ->
                    Tuesday
                when 2 ->
                    Wednesday
                when 3 ->
                    Thursday
                when 4 ->
                    Friday
                when 5 ->
                    Saturday
                when _ ->
                    Sunday

test "weekday epoch is Thursday" =
    assert weekday (fromMillis 0) == Thursday

test "weekday known dates" =
    -- 2024-06-15 is a Saturday
    let result = parse "YYYY-MM-DD" "2024-06-15"
    let wd =
        match result
            when Ok i ->
                weekday i
            when Err _ ->
                Monday
    assert wd == Saturday


-- # Duration constructors

-- | Create a Duration from milliseconds.
export
milliseconds : Int -> Duration
milliseconds n = Duration n

-- | Create a Duration from seconds.
export
seconds : Int -> Duration
seconds n = Duration (n * 1000)

-- | Create a Duration from minutes.
export
minutes : Int -> Duration
minutes n = Duration (n * 60000)

-- | Create a Duration from hours.
export
hours : Int -> Duration
hours n = Duration (n * 3600000)

-- | Create a Duration from days.
export
days : Int -> Duration
days n = Duration (n * 86400000)

test "duration constructors" =
    assert toMilliseconds (milliseconds 500) == 500
    assert toMilliseconds (seconds 3) == 3000
    assert toMilliseconds (minutes 2) == 120000
    assert toMilliseconds (hours 1) == 3600000
    assert toMilliseconds (days 1) == 86400000


-- # Duration accessors

-- | Extract the total milliseconds from a Duration.
export
toMilliseconds : Duration -> Int
toMilliseconds d =
    match d
        when Duration ms ->
            ms

-- | Convert a Duration to total seconds (truncated).
export
toSeconds : Duration -> Int
toSeconds d =
    match d
        when Duration ms ->
            ms / 1000


-- # Instant arithmetic

-- | Add a Duration to an Instant.
export
add : Duration -> Instant -> Instant
add d instant =
    match (d, instant)
        when (Duration dms, Instant ims) ->
            Instant (ims + dms)

test "add duration" =
    let i = fromMillis 1000
    let result = i |> add (seconds 5)
    assert toMillis result == 6000

test "add with pipe chain" =
    let result =
        fromMillis 0
            |> add (hours 1)
            |> add (minutes 30)
    assert toMillis result == 5400000


-- | Subtract a Duration from an Instant.
export
sub : Duration -> Instant -> Instant
sub d instant =
    match (d, instant)
        when (Duration dms, Instant ims) ->
            Instant (ims - dms)

test "sub duration" =
    let i = fromMillis 10000
    let result = i |> sub (seconds 5)
    assert toMillis result == 5000


-- | Compute the Duration between two Instants (later - earlier).
export
diff : Instant -> Instant -> Duration
diff earlier later =
    match (earlier, later)
        when (Instant ems, Instant lms) ->
            Duration (lms - ems)

test "diff instants" =
    let a = fromMillis 1000
    let b = fromMillis 6000
    assert toMilliseconds (diff a b) == 5000
    assert toSeconds (diff a b) == 5


-- # Trait instances

impl Show Instant where
    show instant =
        match instant
            when Instant ms ->
                formatParts "YYYY-MM-DDTHH:mm:ss" (millisToParts ms 0)

impl Eq Instant where
    eq a b = toMillis a == toMillis b

impl Ord Instant where
    compare a b = compare (toMillis a) (toMillis b)

impl Show Duration where
    show d =
        match d
            when Duration ms ->
                "${ms}ms"

impl Eq Duration where
    eq a b = toMilliseconds a == toMilliseconds b

impl Ord Duration where
    compare a b = compare (toMilliseconds a) (toMilliseconds b)

impl Show Weekday where
    show w =
        match w
            when Monday ->
                "Monday"
            when Tuesday ->
                "Tuesday"
            when Wednesday ->
                "Wednesday"
            when Thursday ->
                "Thursday"
            when Friday ->
                "Friday"
            when Saturday ->
                "Saturday"
            when Sunday ->
                "Sunday"

impl Eq Weekday where
    eq a b = show a == show b

test "show instant" =
    let i = fromMillis 0
    assert show i == "1970-01-01T00:00:00"

test "show duration" =
    assert show (seconds 5) == "5000ms"

test "show weekday" =
    assert show Monday == "Monday"
    assert show Sunday == "Sunday"

test "instant equality" =
    assert fromMillis 1000 == fromMillis 1000
    assert not (fromMillis 1000 == fromMillis 2000)

test "duration equality" =
    assert seconds 5 == milliseconds 5000
