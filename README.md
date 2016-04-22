# WordPress Importer ( wpi )

## Protocol flow

```
# Authenticate 

> AUTH generated_token_from_ide\n
< 0\n # auth ok
< 1\n # auth failed

# Set properties ( bellow properties are requred )

> SET url http://myblog.com\n
< OK\n
> SET files 1000\n
< OK\n

# Import files ( this cycle can be repeated for all files )

> IMPORT file/path/name.ext|123456\n
< OK
{file_data}\n
> END\n
< OK\n

# Finish after import

> FINISH\n
< FINISH\n
```
