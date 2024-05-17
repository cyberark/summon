FROM python:3.12

RUN mkdir /src
WORKDIR /src

COPY deploy.py /src/

ENTRYPOINT [ "python", "deploy.py" ]

