export external print : a -> a

export external println : a -> a

export external readLine : String -> String

export external readFile : String -> Result String String

export external writeFile : String -> String -> Result () String

export external appendFile : String -> String -> Result () String

export external fileExists : String -> Bool

export external listDir : String -> Result [String] String
