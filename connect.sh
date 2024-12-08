#!/bin/bash

VM_NR=$1

echo "connecting to VM ${VM_NR}"

BASTION_HOST_NAME=$(terraform -chdir=infra output -json bastion_host_name | jq -r)

RESOURCE_GROUP_NAME=$(terraform -chdir=infra output -json resource_group_name | jq -r)

readarray -t VM_RESOURCE_IDS < <(terraform -chdir=infra output -json vm_resource_ids | jq -r '.[].[]')

chmod 400 ./infra/private_keys/cloudtls.pem

az network bastion ssh --name ${BASTION_HOST_NAME} --resource-group $RESOURCE_GROUP_NAME --target-resource-id ${VM_RESOURCE_IDS[VM_NR]} --auth-type "ssh-key" --username azureuser --ssh-key ./infra/private_keys/cloudtls.pem
