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

# Files to save between steps for CI
CI_SAVE_FILES := aws_credentials cluster_name
CI_SAVE_INSTALL_FILES := metadata.json terraform.cluster.tfstate
CI_SAVE_TF_FILES := terraform.tfstate
# Files to save as artifacts of runs
CI_ARTIFACTS := .oc-mirror.log install/.openshift_install.log

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
.PHONY: clean

realclean: clean
	rm -rf output/*
.PHONY: realclean

###############################################################################
# The following targets are only intended for use in OpenShift CI systems.    #
###############################################################################
_ci:
	@if [ -z "$${OPENSHIFT_CI}" ]; then echo "Not running in CI environment!"; exit 1; fi
.PHONY: _ci

ci: _ci
	@function ci_save() {
		make ci-artifacts ci-save
	}
	@trap ci_save EXIT
	@echo "Sourcing CI environment."; source .env; make create test PYTHON=$(PYTHON)
.PHONY: ci

ci-delete: _ci ci-recover
	@echo "Sourcing CI environment."; source .env; make delete PYTHON=$(PYTHON)
.PHONY: ci-delete

ci-save: _ci
	@echo "Saving shared files."
	@function save() {
		if [ -f "$$1" ]; then cp "$$1" "$${SHARED_DIR}/"; fi
	}
	@for file in $(CI_SAVE_FILES); do save "output/$$file"; done
	@for file in $(CI_SAVE_INSTALL_FILES); do save "output/install/$$file"; done
	@for file in $(CI_SAVE_TF_FILES); do save "output/terraform/$$file"; done
.PHONY: ci-save

ci-recover: _ci
	@echo "Recovering shared files."
	@function recover() {
		local rec_file
		rec_file="$$(basename "$$1")"
		if [ -f "$${SHARED_DIR}/$$rec_file" ]; then cp "$${SHARED_DIR}/$$rec_file" "$$1"; fi
	}
	@for file in $(CI_SAVE_FILES); do recover "output/$$file"; done
	@for file in $(CI_SAVE_INSTALL_FILES); do recover "output/install/$$file"; done
	@for file in $(CI_SAVE_TF_FILES); do recover "output/terraform/$$file"; done
.PHONY: ci-recover

ci-artifacts: _ci
	@echo "Recovering artifacts."
	@function recover() {
		if [ -f "$$1" ]; then cp "$$1" "$${ARTIFACT_DIR}/"; fi
	}
	for file in $(CI_ARTIFACTS); do recover "$$file"; done
.PHONY: ci-artifacts

/usr/bin/python3.9:
	dnf -y install python39

ci-prereqs: _ci /usr/bin/python3.9 terraform collection
	source .venv/bin/activate && ansible-playbook \
		jharmison_redhat.oc_mirror_e2e.terraform_setup \
		-e "output_dir=${PWD}/output"
.PHONY: ci-prereqs

ci-mock: /usr/bin/podman
	pushd ../..
	podman build . -f images/ci/Dockerfile.integration -t oc-mirror-ci-mock
	popd
	mkdir -p ci/artifacts
	mkdir -p ci/shared
	function ci_run() {
		podman run --rm -it -e OPENSHIFT_CI=true -e SHARED_DIR="/shared" -e ARTIFACT_DIR="/artifacts" -v ./ci/artifacts:/artifacts -v ./ci/shared:/shared --security-opt=label=disable --privileged --user 1000 oc-mirror-ci-mock "$$@"
	}
	function ci_cleanup() {
		ci_run make ci-delete
	}
	trap ci_cleanup EXIT
	ci_run make ci
.PHONY: ci-mock
