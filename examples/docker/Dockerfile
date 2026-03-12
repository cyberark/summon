FROM python:3.14

RUN mkdir /src
WORKDIR /src

COPY deploy.py /src/

ENTRYPOINT [ "python", "deploy.py" ]

