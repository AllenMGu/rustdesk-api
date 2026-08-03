# RDGen development setup

Production should use the repository root Full S6 image. The commands below
run only the embedded Django component for development.

## Repository configuration

In the same `rustdesk-api` GitHub repository:

1. Add the Actions secret `ZIP_PASSWORD`.
2. Create a fine-grained token restricted to this repository with
   `Actions: Read and write` and `Contents: Read and write`.
3. Set `GHUSER` to the repository owner and `REPONAME` to `rustdesk-api` (or
   the name of your fork).

The local `ZIP_PASSWORD` value must exactly match the repository Actions
secret. Do not commit the token, password, or Django secret.

## Docker Compose

From the repository root:

```sh
cd rdgen
export SECRET_KEY="$(openssl rand -hex 32)"
export GHUSER=AllenMGu
export REPONAME=rustdesk-api
export GHBEARER='your-fine-grained-token'
export ZIP_PASSWORD='the-actions-secret-value'
export ALLOWED_HOSTS='localhost,127.0.0.1'
docker compose up -d --build
```

The development Compose file publishes `127.0.0.1:8000` only. Put an
authenticated reverse proxy in front of it before allowing remote access.

## Python virtual environment

```sh
cd rdgen
python -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
export SECRET_KEY="$(openssl rand -hex 32)"
export GHUSER=AllenMGu
export REPONAME=rustdesk-api
export GHBEARER='your-fine-grained-token'
export ZIP_PASSWORD='the-actions-secret-value'
export ALLOWED_HOSTS='localhost,127.0.0.1'
python manage.py migrate
python manage.py runserver 127.0.0.1:8000
```

Run the poller in another activated shell when testing the outbound Artifact
flow:

```sh
python manage.py poll_github_artifacts
```
