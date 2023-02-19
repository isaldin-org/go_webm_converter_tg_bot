## Preparations
Rename `.dev.env` to `.env` file and fill with necessary values

## Run
### Docker
Build image \
`docker build -t shebm_bot .`

Run image \
`docker run --env-file .env -d --restart on-failure -v $(pwd)/boltdb_files:/app/boltdb_files shebm_bot`

### k8s (minikube)
`task deploy`