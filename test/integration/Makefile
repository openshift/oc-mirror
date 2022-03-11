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

all:
	thing=$$(cat /aws-creds/AWS_FAKE_ACCESS_KEY) && [ "$$thing" = "fannypack" ] && echo worked || echo failed
	cat /aws-creds/AWS_FAKE_ACCESS_KEY
	exit 1
.PHONY: all

default-scenario: create test delete
.PHONY: default-scenario

.venv/bin/pip:
	$(PYTHON) -m venv .venv
	.venv/bin/pip install --upgrade pip setuptools wheel

.venv/bin/ansible-galaxy: .venv/bin/pip requirements.txt
	.venv/bin/pip install -r requirements.txt

collections/ansible_collections/jharmison_redhat/oc_mirror_e2e: .venv/bin/ansible-galaxy requirements.yml
	.venv/bin/ansible-galaxy install -r requirements.yml

collection: collections/ansible_collections/jharmison_redhat/oc_mirror_e2e
.PHONY: collection

bin/terraform:
	mkdir -p bin
	cd bin \
	&& curl -sLO https://releases.hashicorp.com/terraform/$(TERRAFORM_VERSION)/$(TERRAFORM_DOWNLOAD) \
	&& unzip $(TERRAFORM_DOWNLOAD)

terraform: bin/terraform
.PHONY: terraform

create: terraform collection
	@.venv/bin/ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.create \
		-e @vars.yml \
		-e "output_dir=${PWD}/output" \
		-e "terraform_binary_path=${PWD}/bin/terraform" \
		-e '{"console_redhat_com_pull_secret": $(CONSOLE_REDHAT_COM_PULL_SECRET)}' \
		$(ANSIBLE_PLAYBOOK_ARGS)
.PHONY: create

test: collection
	@.venv/bin/ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.test \
		-e @vars.yml \
		-e "output_dir=${PWD}/output" \
		-e "terraform_binary_path=${PWD}/bin/terraform" \
		-e '{"console_redhat_com_pull_secret": $(CONSOLE_REDHAT_COM_PULL_SECRET)}' \
		$(ANSIBLE_PLAYBOOK_ARGS)
.PHONY: test

delete: terraform collection
	@.venv/bin/ansible-playbook \
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
.PHONY: clean

realclean: clean
	rm -rf output/*
.PHONY: realclean
