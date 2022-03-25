# To enable us to do cleanup on errors
export SHELL:=/bin/bash
export SHELLOPTS:=$(if $(SHELLOPTS),$(SHELLOPTS):)pipefail:errexit
.ONESHELL:

# For virtualenv/ansible installation
PYTHON := python3

# For terraform download
TERRAFORM_VERSION := 1.1.6
TERRAFORM_OS := linux
TERRAFORM_ARCH := amd64
TERRAFORM_DOWNLOAD = terraform_$(TERRAFORM_VERSION)_$(TERRAFORM_OS)_$(TERRAFORM_ARCH).zip

# For Red Hat Quay installation and OpenShift release artifact mirroring
CONSOLE_REDHAT_COM_PULL_SECRET := ${CONSOLE_REDHAT_COM_PULL_SECRET}

# Extra args to pass to a playbook call
# e.g. make test ANSIBLE_PLAYBOOK_ARGS="--tags sneakernet"
ANSIBLE_PLAYBOOK_ARGS :=

all: default-scenario
.PHONY: all

default-scenario: terraform collection
	# This allows us to force tear_down even when a test fails, but still report failure
	# It is enabled by .ONESHELL above
	@function tear_down() {
		make delete
	}
	@trap tear_down EXIT
	@make create test
.PHONY: default-scenario

.venv/bin/pip:
	$(PYTHON) -m venv .venv
	source .venv/bin/activate && pip install --upgrade pip setuptools wheel

.venv/bin/ansible-galaxy: .venv/bin/pip requirements.txt
	source .venv/bin/activate && pip install -r requirements.txt

collections/ansible_collections/jharmison_redhat/oc_mirror_e2e: .venv/bin/ansible-galaxy requirements.yml
	source .venv/bin/activate && ansible-galaxy install -r requirements.yml

collection: collections/ansible_collections/jharmison_redhat/oc_mirror_e2e
.PHONY: collection

bin/terraform:
	mkdir -p bin
	cd bin && \
		curl -sLO https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/$(TERRAFORM_DOWNLOAD) && \
		unzip $(TERRAFORM_DOWNLOAD) && \
		rm -f $(TERRAFORM_DOWNLOAD) && \
		cd ..

terraform: bin/terraform
.PHONY: terraform

create:
	@echo "Running jharmison_redhat.oc_mirror_e2e.create"
	@source .venv/bin/activate && ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.create \
		-e @vars.yml \
		-e "output_dir=${PWD}/output" \
		-e "terraform_binary_path=${PWD}/bin/terraform" \
		-e '{"console_redhat_com_pull_secret": $(CONSOLE_REDHAT_COM_PULL_SECRET)}' \
		$(ANSIBLE_PLAYBOOK_ARGS)
.PHONY: create

test:
	@echo "Running jharmison_redhat.oc_mirror_e2e.test"
	@source .venv/bin/activate && ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.test \
		-e @vars.yml \
		-e "output_dir=${PWD}/output" \
		-e "terraform_binary_path=${PWD}/bin/terraform" \
		-e '{"console_redhat_com_pull_secret": $(CONSOLE_REDHAT_COM_PULL_SECRET)}' \
		$(ANSIBLE_PLAYBOOK_ARGS)
.PHONY: test

delete:
	@echo "Running jharmison_redhat.oc_mirror_e2e.delete"
	@source .venv/bin/activate && ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.delete \
		-e @vars.yml \
		-e "output_dir=${PWD}/output" \
		-e "terraform_binary_path=${PWD}/bin/terraform" \
		$(ANSIBLE_PLAYBOOK_ARGS)
.PHONY: delete

clean:
	rm -rf .venv
	rm -rf bin
	rm -rf collections
	rm -rf ci
.PHONY: clean

realclean: clean
	rm -rf output/*
.PHONY: realclean

###############################################################################
# The following targets are only intended for use in OpenShift CI systems.    #
###############################################################################
ci:
	scripts/run.sh
.PHONY: ci

ci-delete:
	scripts/run.sh delete
.PHONY: ci-delete
