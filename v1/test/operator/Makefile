PYTHON := python3
PUBLISH_FLAGS :=

all: publish
.PHONY: all

.venv/bin/pip:
	@echo "Creating virtual environment" >&2
	@$(PYTHON) -m venv .venv
	@.venv/bin/pip install --upgrade pip setuptools wheel

.requirements: .venv/bin/pip requirements.txt
	@echo "Installing requirements" >&2
	@.venv/bin/pip install -r requirements.txt
	@touch .requirements

publish: .requirements publish_images.py
	@echo "Rendering and publishing images" >&2
	@.venv/bin/python publish_images.py $(PUBLISH_FLAGS)
.PHONY: publish
