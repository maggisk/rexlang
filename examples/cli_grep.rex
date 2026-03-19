-- cli_grep.rex — A simple grep-like CLI tool
--
-- Usage:  rex examples/cli_grep.rex <pattern> <file1> [file2 ...]
--
-- Searches each file for lines containing the pattern and prints
-- matching lines with file name and line number. Returns 0 if any
-- matches were found, 1 if none.

import Std:IO (println, readFile)
import Std:String (contains, split, toString, padLeft)
import Std:List (filter, indexedMap, concat)
import Std:Result (Ok, Err)


-- A single match: which file, which line number, and the line text.
type Match = { file : String, lineNum : Int, text : String }


-- | Format a match for display: "file:line: text"
formatMatch : Match -> String
formatMatch m =
    let num = m.lineNum |> toString |> padLeft 4 " "
    in "${m.file}:${num}: ${m.text}"


-- | Search a single file for lines containing the pattern.
-- Returns a list of Match records.
searchFile : String -> String -> [Match]
searchFile pattern filename =
    match readFile filename
        when Err e ->
            let _ = println "Error reading ${filename}: ${e}"
            in []
        when Ok content ->
            content
                |> split "\n"
                |> indexedMap (\i line -> (i + 1, line))
                |> filter (\pair ->
                    let (_, line) = pair
                    in contains pattern line)
                |> indexedMap (\_ pair ->
                    let (num, line) = pair
                    in Match { file = filename, lineNum = num, text = line })


-- | Search multiple files and collect all matches.
searchFiles : String -> [String] -> [Match]
searchFiles pattern files =
    files
        |> indexedMap (\_ f -> searchFile pattern f)
        |> concat


-- | Print all matches and return whether any were found.
printMatches : [Match] -> Bool
printMatches matches =
    match matches
        when [] ->
            false
        when _ ->
            let _ = matches
                    |> indexedMap (\_ m -> println (formatMatch m))
            in true


export
main : [String] -> Int
main args =
    match args
        when [pattern, file] ->
            -- Single file
            let found = searchFile pattern file |> printMatches
            in if found then
                0
            else
                1
        when [pattern, file | rest] ->
            -- Multiple files
            let found = searchFiles pattern (file :: rest) |> printMatches
            in if found then
                0
            else
                1
        when _ ->
            let _ = println "Usage: rex cli_grep.rex <pattern> <file1> [file2 ...]"
            in 1
