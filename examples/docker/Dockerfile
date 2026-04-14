FROM python:3.15-rc-slim-trixie

RUN mkdir /src
WORKDIR /src

COPY deploy.py /src/

ENTRYPOINT [ "python", "deploy.py" ]

