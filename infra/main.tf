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

# Create network interface
# resource "azurerm_network_interface" "bastion_nic" {
#   name                = "bastion_nic"
#   location            = azurerm_resource_group.rg.location
#   resource_group_name = azurerm_resource_group.rg.name

#   ip_configuration {
#     name                          = "my_nic_configuration"
#     subnet_id                     = azurerm_subnet.my_terraform_subnet.id
#     private_ip_address_allocation = "Dynamic"
#     public_ip_address_id          = azurerm_public_ip.my_terraform_public_ip.id
#   }
# }

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
  count               = 2
  name                = "nic_${count.index}"
  location            = azurerm_resource_group.rg.location
  resource_group_name = azurerm_resource_group.rg.name

  ip_configuration {
    name                          = "my_nic_configuration"
    subnet_id                     = azurerm_subnet.my_terraform_subnet.id
    private_ip_address_allocation = "Dynamic"
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
    source_address_prefix      = "10.1.1.0/24"
    destination_address_prefix = "*"
  }
}

# Connect the security group to the network interface
# resource "azurerm_network_interface_security_group_association" "bastion_host" {
#   network_interface_id      = azurerm_network_interface.bastion_nic.id
#   network_security_group_id = azurerm_network_security_group.my_terraform_nsg.id
# }

# Connect the security group to the network interface
resource "azurerm_network_interface_security_group_association" "example" {
  count                     = 2
  network_interface_id      = element(azurerm_network_interface.my_terraform_nic.*.id, count.index)
  network_security_group_id = azurerm_network_security_group.my_terraform_nsg.id
}

# Create (and display) an SSH key
resource "tls_private_key" "example_ssh" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Create virtual machine
# resource "azurerm_linux_virtual_machine" "bastion_host_terraform_vm" {
#   name                            = "bastion_host_vm"
#   location                        = azurerm_resource_group.rg.location
#   resource_group_name             = azurerm_resource_group.rg.name
#   network_interface_ids           = [azurerm_network_interface.bastion_nic.id]
#   size                            = "Standard_B1s"
#   computer_name                   = "myvm"
#   admin_username                  = "azureuser"
#   disable_password_authentication = true

#   admin_ssh_key {
#     username   = "azureuser"
#     public_key = tls_private_key.example_ssh.public_key_openssh
#   }

#   os_disk {
#     caching              = "ReadWrite"
#     storage_account_type = "Premium_LRS"
#     name                 = "myosdisk_bastion"
#   }

#   source_image_reference {
#     publisher = "Canonical"
#     offer     = "0001-com-ubuntu-server-jammy"
#     sku       = "22_04-lts-gen2"
#     version   = "latest"
#   }
# }

resource "random_pet" "azurerm_linux_virtual_machine_name" {
  prefix = "vm"
}

# Create virtual machine
resource "azurerm_linux_virtual_machine" "my_terraform_vm" {
  count                           = 2
  name                            = "${random_pet.azurerm_linux_virtual_machine_name.id}${count.index}"
  location                        = azurerm_resource_group.rg.location
  resource_group_name             = azurerm_resource_group.rg.name
  network_interface_ids           = [azurerm_network_interface.my_terraform_nic[count.index].id]
  size                            = "Standard_B1s"
  computer_name                   = "myvm"
  admin_username                  = "azureuser"
  disable_password_authentication = true

  admin_ssh_key {
    username   = "azureuser"
    public_key = tls_private_key.example_ssh.public_key_openssh
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Premium_LRS"
    name                 = "myosdisk_${count.index}"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }
}


resource "azurerm_managed_disk" "test" {
  count                = 2
  name                 = "datadisk_existing_${count.index}"
  location             = azurerm_resource_group.rg.location
  resource_group_name  = azurerm_resource_group.rg.name
  storage_account_type = "Premium_LRS"
  create_option        = "Empty"
  disk_size_gb         = "1024"
}

resource "azurerm_virtual_machine_data_disk_attachment" "test" {
  count              = 2
  managed_disk_id    = azurerm_managed_disk.test[count.index].id
  virtual_machine_id = azurerm_linux_virtual_machine.my_terraform_vm[count.index].id
  lun                = "10"
  caching            = "ReadWrite"
}

resource "local_file" "cloud_pem" {
  filename = "${path.module}/private_keys/cloudtls.pem"
  content  = tls_private_key.example_ssh.private_key_pem
}
