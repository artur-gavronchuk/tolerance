FROM python:3.12-alpine
RUN pip install --no-cache-dir pytest==8.3.3
# No PYTHONPATH: /work must never precede site-packages when pytest itself is
# imported. Plugin autoload is off so dist-info entry points in /work cannot
# load a plugin; `python -P` (run_cmd) keeps the cwd off sys.path.
ENV PYTHONDONTWRITEBYTECODE=1 PYTEST_DISABLE_PLUGIN_AUTOLOAD=1
WORKDIR /work
