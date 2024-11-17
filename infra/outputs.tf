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

output "bastion_host_name" {
  value= azurerm_bastion_host.example.name
}

output "vm_resource_ids" {
  value = ["${azurerm_linux_virtual_machine.my_terraform_vm.*.id}"]
}

output "local_pem_file" {
  value = local_file.cloud_pem.filename
}
