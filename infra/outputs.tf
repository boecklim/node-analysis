output "resource_group_name" {
  value = azurerm_resource_group.rg.name
}

output "private_ip_address" {
  value = ["${azurerm_linux_virtual_machine.my_terraform_vm.*.private_ip_address}"]
}


output "public_ip_address" {
  value = azurerm_public_ip.my_terraform_public_ip.ip_address
}

output "tls_private_key" {
  value     = tls_private_key.example_ssh.private_key_pem
  sensitive = true
}
