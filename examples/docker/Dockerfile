FROM python:3.11.0

RUN mkdir /src
WORKDIR /src

COPY deploy.py /src/

ENTRYPOINT [ "python", "deploy.py" ]

