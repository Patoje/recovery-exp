# Cada objetivo corresponde a un script en scripts/. Ver README.md.
# RUN_DIR por defecto: la corrida indicada en runs/CURRENT (la crea `make up`).
SHELL := /bin/bash
SNAPSHOT ?= latest

.PHONY: reset up seed witness-pre backup inventory witness-post restore b0 run clean-runs

reset:        ## borra contenedores, redes y volúmenes (incluye repo Restic)
	scripts/reset.sh

up:           ## nueva corrida: levanta Gitea + PostgreSQL, admin y repo Restic
	scripts/up.sh

seed:         ## carga el dataset determinista
	scripts/seed.sh

witness-pre:  ## testigos previos al backup
	scripts/witness.sh pre

backup:       ## backup de referencia, con inventario tomado con Gitea detenido
	INVENTORY_HOOK=$(CURDIR)/scripts/inventory.sh scripts/backup.sh

inventory:    ## solo el inventario (fuera de la ventana de backup)
	scripts/inventory.sh

witness-post: ## un testigo por minuto hasta T = INCIDENT_MINUTES
	scripts/witness.sh post

restore:      ## restauración de referencia aislada (SNAPSHOT=latest|<id>)
	scripts/restore.sh $(SNAPSHOT)

b0:           ## línea base B0 (incluye restic check y restauración)
	scripts/b0.sh $(SNAPSHOT)

run:          ## corrida completa (después de make reset)
	scripts/run.sh

clean-runs:   ## borra los resultados de corridas anteriores
	rm -rf runs/*/ runs/CURRENT
