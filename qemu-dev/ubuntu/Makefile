# Makefile for bpfman development environment setup

.PHONY: help setup check-vm clean

help:
	@echo "bpfman Ubuntu Development Environment Setup"
	@echo ""
	@echo "Targets:"
	@echo "  setup      Deploy bpfman dev environment to VM"
	@echo "  check-vm   Test SSH connection to VM"
	@echo "  clean      Remove generated inventory file"

setup: inventory.ini
	ansible-playbook -i inventory.ini bpfman-dev-setup.yml

check-vm: inventory.ini
	ansible all -i inventory.ini -m ping

inventory.ini:
	@if [ ! -f inventory.ini ]; then \
		cp inventory.ini.example inventory.ini; \
	fi

clean:
	rm -f inventory.ini