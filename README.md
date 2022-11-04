## Preparations
Create `.env` file and fill necessary values
```
TOKEN=<bot_token>
DEBUG=<true if going to debug>
ALLOWED_CHAT_ID=<allowed chat id>
```

## Run
Build image \
`docker build -t shebm_bot .`

Run image \
`docker run --env-file .env -d --restart on-failure -v $(pwd)/boltdb_files:/app/boltdb_files shebm_bot`