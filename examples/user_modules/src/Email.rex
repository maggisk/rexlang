-- Email module: opaque type example
-- The Email constructor is hidden — consumers must use `make` and `toString`.

export opaque type Email = Email String

export
make : String -> Email
make s = Email s

export
toString : Email -> String
toString e =
    match e
        when Email s ->
            s
