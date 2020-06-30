FROM python:alpine3.10

RUN mkdir /src
WORKDIR /src

COPY deploy.py /src/

ENTRYPOINT [ "python", "deploy.py" ]

