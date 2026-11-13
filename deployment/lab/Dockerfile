FROM python:3.12.12-alpine3.22@sha256:848ba4413eb897e225159b8fc1b02094576cbae4aa73fc13142608ae2c8c0e32
COPY fixture.py /fixture.py
COPY netns.sh /netns.sh
COPY --chmod=0555 webhook_receiver.py /webhook_receiver.py
ENV PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
ENTRYPOINT ["python3", "/fixture.py"]
