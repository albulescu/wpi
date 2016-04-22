# WordPress Importer ( wpi )

## Protocol flow

```
# Authenticate

> AUTH generated_token_from_ide\n
< 0\n # auth ok
< 1\n # auth failed

# Set properties ( bellow properties are requred )

> SET url http://myblog.com\n
< 0\n
> SET files 1000\n
< 0\n

# Import files ( this cycle can be repeated for all files )

> IMPORT file/path/name.ext|123456\n
< 0\n
{file_data}\n
< 0\n

# Finish after import

> FINISH\n
< FINISH\n
```
