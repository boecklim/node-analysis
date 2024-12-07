#!/bin/bash

usage() {
    echo "Usage: $0 [--help] [--build]"
    echo "  --help   Show help"
    echo "  --build  Build the app"
    exit 1
}

for arg in "$@"; do
    case "$arg" in
        --help)
            usage
            ;;
        --build)
            # Build listener
            go build -o build/listener cmd/listener/main.go

            # Build broadcaster
            go build -o build/broadcaster cmd/broadcaster/main.go
            exit 0
            ;;
        *)
            echo "Invalid option: $arg" >&2
            usage
            ;;
    esac
done


# Get bastion host name, resource group, VM resource IDs

BASTION_HOST_NAME=$(terraform -chdir=infra output -json bastion_host_name | jq -r)
echo $BASTION_HOST_NAME

RESOURCE_GROUP_NAME=$(terraform -chdir=infra output -json resource_group_name | jq -r)
echo $RESOURCE_GROUP_NAME

# LOCAL_PEM_FILE=$(terraform -chdir=infra output -json local_pem_file | jq -r)
# echo $LOCAL_PEM_FILE

readarray -t VM_RESOURCE_IDS < <(terraform -chdir=infra output -json vm_resource_ids | jq -r '.[].[]')

# echo ${VM_RESOURCE_IDS[0]}
# echo ${VM_RESOURCE_IDS[1]}


# For each VM 
#  - cteate tunnel
#  - scp binaries
#  - upload & unpack bitcoin node
#  - upload bitcoin.conf to ~/.bitcoin folder

nrOfIDs=${#VM_RESOURCE_IDS[@]}


upload_port=9000

for id in  $(seq 1 $nrOfIDs)
do
    VM_RESOURCE_ID=${VM_RESOURCE_IDS[$id -1]}

    while lsof -Pi ":${upload_port}" -sTCP:LISTEN -t >/dev/null; do
        echo "port ${upload_port} is used currently"
        upload_port=$((upload_port+1))
        sleep 1  # Check periodically
    done

    echo "Creating tunnel"
    # Create tunnel
    az network bastion tunnel --name $BASTION_HOST_NAME --resource-group $RESOURCE_GROUP_NAME --target-resource-id ${VM_RESOURCE_ID} --resource-port 22 --port $upload_port >blocking_output.log 2>&1 &
    BLOCKING_PID=$!  # 

    echo "Waiting for the blocking command to start..."
    while ! grep -q "Tunnel is ready, connect on port ${upload_port}" blocking_output.log; do
        sleep 0.5  # Check periodically
    done

    ssh-keygen -f ~/.ssh/known_hosts -R "[127.0.0.1]:${upload_port}"
    scp -o StrictHostKeyChecking=no -i ./infra/private_keys/cloudtls.pem -P $upload_port ./build/broadcaster azureuser@127.0.0.1:/home/azureuser/

    echo "Stopping the blocking command (PID: $BLOCKING_PID)..."
    kill $BLOCKING_PID

    wait $BLOCKING_PID 2>/dev/null

done

# pkill -P $$ # kill all subprocesses

echo "All tasks completed!"

