resource "random_pet" "rg_name" {
  prefix = var.resource_group_name_prefix
}

resource "azurerm_resource_group" "rg" {
  name     = random_pet.rg_name.id
  location = var.resource_group_location
}

# Create virtual network
resource "azurerm_virtual_network" "my_terraform_network" {
  name                = "myVnet"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
}

# Create subnet
resource "azurerm_subnet" "my_terraform_subnet" {
  name                 = "mySubnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.my_terraform_network.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_subnet" "bastion_subnet" {
  name                 = "AzureBastionSubnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.my_terraform_network.name
  address_prefixes     = ["10.0.2.0/25"]
}

resource "azurerm_bastion_host" "example" {
  name                = "bastion_host"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  sku                 = "Standard"
  tunneling_enabled   = true

  ip_configuration {
    name                 = "configuration"
    subnet_id            = azurerm_subnet.bastion_subnet.id
    public_ip_address_id = azurerm_public_ip.my_terraform_public_ip.id
  }
}

# Create public IPs
resource "azurerm_public_ip" "my_terraform_public_ip" {
  name                = "pip-bastion"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

# Create network interface
resource "azurerm_network_interface" "my_terraform_nic" {
  count               = var.virtual_machines
  name                = "nic_${count.index}"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name

  ip_configuration {
    name                          = "my_nic_configuration"
    subnet_id                     = azurerm_subnet.my_terraform_subnet.id
    private_ip_address_allocation = "Static"
    private_ip_address            = "10.0.1.${count.index + 5}"
  }
}

# Create Network Security Group and rule
resource "azurerm_network_security_group" "my_terraform_nsg" {
  name                = "myNetworkSecurityGroup"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name

  security_rule {
    name                       = "SSH"
    priority                   = 1001
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }

  security_rule {
    name                       = "AllowCidrBlockCustom18444Inbound"
    priority                   = 800
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "18444"
    source_address_prefix      = "10.0.1.0/24"
    destination_address_prefix = "*"
  }
}


# Connect the security group to the network interface
resource "azurerm_network_interface_security_group_association" "example" {
  count                     = var.virtual_machines
  network_interface_id      = element(azurerm_network_interface.my_terraform_nic.*.id, count.index)
  network_security_group_id = azurerm_network_security_group.my_terraform_nsg.id
}

# Create (and display) an SSH key
resource "tls_private_key" "example_ssh" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "random_pet" "azurerm_linux_virtual_machine_name" {
  prefix = "vm"
}

# Create virtual machine
resource "azurerm_linux_virtual_machine" "my_terraform_vm" {
  count                           = var.virtual_machines
  name                            = "${random_pet.azurerm_linux_virtual_machine_name.id}${count.index}"
  location                        = azurerm_resource_group.rg.location
  resource_group_name             = azurerm_resource_group.rg.name
  network_interface_ids           = [azurerm_network_interface.my_terraform_nic[count.index].id]
  size                            = var.vm_size
  computer_name                   = "myvm${count.index}"
  admin_username                  = "azureuser"
  disable_password_authentication = true

  admin_ssh_key {
    username   = "azureuser"
    public_key = tls_private_key.example_ssh.public_key_openssh
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
    name                 = "myosdisk_${count.index}"
  }

  source_image_reference { # alternative images: https://documentation.ubuntu.com/azure/en/latest/azure-how-to/instances/find-ubuntu-images/
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server-gen1"
    version   = "latest"
  }
  
  custom_data = var.use_btc ? data.template_cloudinit_config.config_btc.rendered : data.template_cloudinit_config.config_bsv.rendered
}

data "template_cloudinit_config" "config_btc" {
  gzip          = true
  base64_encode = true

  # Main cloud-config configuration file.
  part {
    content_type = "text/cloud-config"
    content      = <<EOF
#cloud-config
write_files:
  - owner: azureuser:azureuser
    path: /root/.bitcoin/bitcoin.conf
    defer: true
    content: |
        regtest=1
        debug=1
        listen=1
        rpcuser=bitcoin
        rpcpassword=bitcoin
        zmqpubhashtx=tcp://127.0.0.1:29000
        zmqpubhashblock=tcp://127.0.0.1:29000
        datadir=/home/azureuser/bitcoin-28.0/data
        minrelaytxfee=0
        listenonion=0
        [regtest]
        connect=10.0.1.5
        connect=10.0.1.6
        connect=10.0.1.7
        connect=10.0.1.8
        connect=10.0.1.9
  - owner: azureuser:azureuser
    path: /etc/systemd/system/bitcoin.service
    permissions: '0644'
    content: |
      [Unit]
      Description=Bitcoin Service
      After=network.target

      [Service]
      ExecStart=/home/azureuser/bitcoin-28.0/bin/bitcoind
      Restart=always
      User=root

      [Install]
      WantedBy=multi-user.target
runcmd:
  - echo "Running custom startup commands"
  - apt-get update
  - apt-get install -y wget ca-certificates
  - wget -P /home/azureuser https://bitcoincore.org/bin/bitcoin-core-28.0/bitcoin-28.0-x86_64-linux-gnu.tar.gz
  - cd /home/azureuser
  - tar xzf bitcoin-28.0-x86_64-linux-gnu.tar.gz
  - mkdir /home/azureuser/bitcoin-28.0/data
  - systemctl enable bitcoin
  - systemctl start bitcoin
  - wget -P /home/azureuser https://github.com/boecklim/node-analysis/releases/download/${var.broadcaster_version}/broadcaster
  - sleep 120
  - /home/azureuser/broadcaster -rpc-port=18443 -zmq-port=29000 -blockchain=btc -gen-blocks=${var.gen_block_time} -rate=${var.rate} -limit=${var.limit} -start-at=${var.start_time} -output=/home/azureuser/output.log
EOF
  }
}

data "template_cloudinit_config" "config_bsv" {
  gzip          = true
  base64_encode = true

  # Main cloud-config configuration file.
  part {
    content_type = "text/cloud-config"
    content      = <<EOF
#cloud-config
write_files:
  - owner: azureuser:azureuser
    path: /home/azureuser/bitcoin-sv-1.1.0/bitcoin.conf
    defer: true
    content: |
        server=1
        rest=1
        listen=1
        regtest=1
        printtoconsole=1
        rpcport=18332
        rpcuser=bitcoin
        rpcpassword=bitcoin
        rpcallowip=0.0.0.0/0
        port=18444
        blockmaxsize=512000000
        excessiveblocksize=2000000000
        maxstackmemoryusageconsensus=200000000
        dbcache=16384
        maxsigcachesize=260
        maxscriptcachesize=260
        maxorphantx=100000
        maxmempool=2000
        blockreconstructionextratxn=100000
        banscore=10000
        zmqpubhashblock=tcp://*:28332
        zmqpubhashtx=tcp://*:28332
        genesisactivationheight=1
        minminingtxfee=0.0000005

        connect=10.0.1.5
        connect=10.0.1.6
        connect=10.0.1.7
        connect=10.0.1.8
        connect=10.0.1.9
  - owner: azureuser:azureuser
    path: /etc/systemd/system/bitcoin.service
    permissions: '0644'
    content: |
      [Unit]
      Description=Bitcoin Service
      After=network.target

      [Service]
      ExecStart=/home/azureuser/bitcoin-sv-1.1.0/bin/bitcoind -conf=/home/azureuser/bitcoin-sv-1.1.0/bitcoin.conf
      Restart=always
      User=root

      [Install]
      WantedBy=multi-user.target
runcmd:
  - echo "Running custom startup commands"
  - apt-get update
  - apt-get install -y wget
  - wget -P /home/azureuser https://download.bitcoinsv.io/bitcoinsv/1.1.0/bitcoin-sv-1.1.0-x86_64-linux-gnu.tar.gz
  - cd /home/azureuser
  - tar xzf bitcoin-sv-1.1.0-x86_64-linux-gnu.tar.gz
  - systemctl enable bitcoin
  - systemctl start bitcoin
EOF
  }
}

resource "local_file" "cloud_pem" {
  filename = "${path.module}/private_keys/cloudtls.pem"
  content  = tls_private_key.example_ssh.private_key_pem
}
