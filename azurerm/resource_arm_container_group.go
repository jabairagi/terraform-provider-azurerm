package azurerm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/containerinstance/mgmt/2018-04-01/containerinstance"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/helpers/tf"
	"github.com/terraform-providers/terraform-provider-azurerm/azurerm/utils"
)

func resourceArmContainerGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmContainerGroupCreate,
		Read:   resourceArmContainerGroupRead,
		Delete: resourceArmContainerGroupDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(time.Minute * 30),
			Delete: schema.DefaultTimeout(time.Minute * 30),
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": locationSchema(),

			"resource_group_name": resourceGroupNameSchema(),

			"ip_address_type": {
				Type:             schema.TypeString,
				Optional:         true,
				Default:          "Public",
				ForceNew:         true,
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
				ValidateFunc: validation.StringInSlice([]string{
					"Public",
				}, true),
			},

			"os_type": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
				ValidateFunc: validation.StringInSlice([]string{
					"windows",
					"linux",
				}, true),
			},

			"image_registry_credential": {
				Type:     schema.TypeList,
				Optional: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"server": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.NoZeroValues,
							ForceNew:     true,
						},

						"username": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.NoZeroValues,
							ForceNew:     true,
						},

						"password": {
							Type:         schema.TypeString,
							Required:     true,
							Sensitive:    true,
							ValidateFunc: validation.NoZeroValues,
							ForceNew:     true,
						},
					},
				},
			},

			"tags": tagsForceNewSchema(),

			"restart_policy": {
				Type:             schema.TypeString,
				Optional:         true,
				ForceNew:         true,
				Default:          string(containerinstance.Always),
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
				ValidateFunc: validation.StringInSlice([]string{
					string(containerinstance.Always),
					string(containerinstance.Never),
					string(containerinstance.OnFailure),
				}, true),
			},

			"ip_address": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"fqdn": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"dns_name_label": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"container": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},

						"image": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},

						"cpu": {
							Type:     schema.TypeFloat,
							Required: true,
							ForceNew: true,
						},

						"memory": {
							Type:     schema.TypeFloat,
							Required: true,
							ForceNew: true,
						},

						"port": {
							Type:         schema.TypeInt,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.IntBetween(1, 65535),
						},

						"protocol": {
							Type:             schema.TypeString,
							Optional:         true,
							ForceNew:         true,
							DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
							ValidateFunc: validation.StringInSlice([]string{
								"tcp",
								"udp",
							}, true),
						},

						"environment_variables": {
							Type:     schema.TypeMap,
							Optional: true,
							ForceNew: true,
						},

						"command": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},

						"volume": {
							Type:     schema.TypeList,
							Optional: true,
							ForceNew: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"name": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},

									"mount_path": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},

									"read_only": {
										Type:     schema.TypeBool,
										Optional: true,
										ForceNew: true,
										Default:  false,
									},

									"share_name": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},

									"storage_account_name": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},

									"storage_account_key": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceArmContainerGroupCreate(d *schema.ResourceData, meta interface{}) error {
	ctx := meta.(*ArmClient).StopContext
	client := meta.(*ArmClient).containerGroupsClient

	// container group properties
	resGroup := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)

	// first check if there's one in this subscription requiring import
	resp, err := client.Get(ctx, resGroup, name)
	if err != nil {
		if !utils.ResponseWasNotFound(resp.Response) {
			return fmt.Errorf("Error checking for the existence of Container Group %q (Resource Group %q): %+v", name, resGroup, err)
		}
	}

	if resp.ID != nil {
		return tf.ImportAsExistsError("azurerm_container_group", *resp.ID)
	}

	location := azureRMNormalizeLocation(d.Get("location").(string))
	OSType := d.Get("os_type").(string)
	IPAddressType := d.Get("ip_address_type").(string)
	tags := d.Get("tags").(map[string]interface{})
	restartPolicy := d.Get("restart_policy").(string)

	containers, containerGroupPorts, containerGroupVolumes := expandContainerGroupContainers(d)
	containerGroup := containerinstance.ContainerGroup{
		Name:     &name,
		Location: &location,
		Tags:     expandTags(tags),
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			Containers:    containers,
			RestartPolicy: containerinstance.ContainerGroupRestartPolicy(restartPolicy),
			IPAddress: &containerinstance.IPAddress{
				Type:  &IPAddressType,
				Ports: containerGroupPorts,
			},
			OsType:                   containerinstance.OperatingSystemTypes(OSType),
			Volumes:                  containerGroupVolumes,
			ImageRegistryCredentials: expandContainerImageRegistryCredentials(d),
		},
	}

	if dnsNameLabel := d.Get("dns_name_label").(string); dnsNameLabel != "" {
		containerGroup.ContainerGroupProperties.IPAddress.DNSNameLabel = &dnsNameLabel
	}

	waitCtx, cancel := context.WithTimeout(ctx, d.Timeout(schema.TimeoutCreate))
	defer cancel()
	_, err = client.CreateOrUpdate(waitCtx, resGroup, name, containerGroup)
	if err != nil {
		return err
	}

	read, err := client.Get(ctx, resGroup, name)
	if err != nil {
		return err
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read container group %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmContainerGroupRead(d, meta)
}

func resourceArmContainerGroupRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	ctx := meta.(*ArmClient).StopContext
	containterGroupsClient := client.containerGroupsClient

	id, err := parseAzureResourceID(d.Id())

	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	name := id.Path["containerGroups"]

	resp, err := containterGroupsClient.Get(ctx, resGroup, name)

	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}
		return err
	}

	d.Set("name", name)
	d.Set("resource_group_name", resGroup)
	if location := resp.Location; location != nil {
		d.Set("location", azureRMNormalizeLocation(*location))
	}
	flattenAndSetTags(d, resp.Tags)

	if err := d.Set("image_registry_credential", flattenContainerImageRegistryCredentials(d, resp.ImageRegistryCredentials)); err != nil {
		return fmt.Errorf("Error setting `capabilities`: %+v", err)
	}

	d.Set("os_type", string(resp.OsType))
	if address := resp.IPAddress; address != nil {
		d.Set("ip_address_type", address.Type)
		d.Set("ip_address", address.IP)
		d.Set("dns_name_label", address.DNSNameLabel)
		d.Set("fqdn", address.Fqdn)
	}
	d.Set("restart_policy", string(resp.RestartPolicy))

	if props := resp.ContainerGroupProperties; props != nil {
		containerConfigs := flattenContainerGroupContainers(d, resp.Containers, props.IPAddress.Ports, props.Volumes)
		err = d.Set("container", containerConfigs)
		if err != nil {
			return fmt.Errorf("Error setting `container`: %+v", err)
		}
	}

	return nil
}

func resourceArmContainerGroupDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	ctx := meta.(*ArmClient).StopContext
	containterGroupsClient := client.containerGroupsClient

	id, err := parseAzureResourceID(d.Id())

	if err != nil {
		return err
	}

	// container group properties
	resGroup := id.ResourceGroup
	name := id.Path["containerGroups"]

	waitCtx, cancel := context.WithTimeout(ctx, d.Timeout(schema.TimeoutDelete))
	defer cancel()
	resp, err := containterGroupsClient.Delete(waitCtx, resGroup, name)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			return nil
		}
		return err
	}

	return nil
}

func flattenContainerGroupContainers(d *schema.ResourceData, containers *[]containerinstance.Container, containerGroupPorts *[]containerinstance.Port, containerGroupVolumes *[]containerinstance.Volume) []interface{} {

	containerConfigs := make([]interface{}, 0, len(*containers))
	for _, container := range *containers {
		containerConfig := make(map[string]interface{})
		containerConfig["name"] = *container.Name
		containerConfig["image"] = *container.Image

		if resources := container.Resources; resources != nil {
			if resourceRequests := resources.Requests; resourceRequests != nil {
				containerConfig["cpu"] = *resourceRequests.CPU
				containerConfig["memory"] = *resourceRequests.MemoryInGB
			}
		}

		if len(*container.Ports) > 0 {
			containerPort := *(*container.Ports)[0].Port
			containerConfig["port"] = containerPort
			// protocol isn't returned in container config, have to search in container group ports
			protocol := ""
			if containerGroupPorts != nil {
				for _, cgPort := range *containerGroupPorts {
					if *cgPort.Port == containerPort {
						protocol = string(cgPort.Protocol)
					}
				}
			}
			if protocol != "" {
				containerConfig["protocol"] = protocol
			}
		}

		if container.EnvironmentVariables != nil {
			if len(*container.EnvironmentVariables) > 0 {
				containerConfig["environment_variables"] = flattenContainerEnvironmentVariables(container.EnvironmentVariables)
			}
		}

		if command := container.Command; command != nil {
			containerConfig["command"] = strings.Join(*command, " ")
		}

		if containerGroupVolumes != nil && container.VolumeMounts != nil {
			// Also pass in the container volume config from schema
			var containerVolumesConfig *[]interface{}
			containersConfigRaw := d.Get("container").([]interface{})
			for _, containerConfigRaw := range containersConfigRaw {
				data := containerConfigRaw.(map[string]interface{})
				nameRaw := data["name"].(string)
				if nameRaw == *container.Name {
					// found container config for current container
					// extract volume mounts from config
					if v, ok := data["volume"]; ok {
						containerVolumesRaw := v.([]interface{})
						containerVolumesConfig = &containerVolumesRaw
					}
				}
			}
			containerConfig["volume"] = flattenContainerVolumes(container.VolumeMounts, containerGroupVolumes, containerVolumesConfig)
		}

		containerConfigs = append(containerConfigs, containerConfig)
	}

	return containerConfigs
}

func flattenContainerEnvironmentVariables(input *[]containerinstance.EnvironmentVariable) map[string]interface{} {
	output := make(map[string]interface{})

	for _, envVar := range *input {
		k := *envVar.Name
		v := *envVar.Value

		output[k] = v
	}

	return output
}

func flattenContainerVolumes(volumeMounts *[]containerinstance.VolumeMount, containerGroupVolumes *[]containerinstance.Volume, containerVolumesConfig *[]interface{}) []interface{} {
	volumeConfigs := make([]interface{}, 0)

	for _, vm := range *volumeMounts {
		volumeConfig := make(map[string]interface{})
		volumeConfig["name"] = *vm.Name
		volumeConfig["mount_path"] = *vm.MountPath
		if vm.ReadOnly != nil {
			volumeConfig["read_only"] = *vm.ReadOnly
		}

		// find corresponding volume in container group volumes
		// and use the data
		for _, cgv := range *containerGroupVolumes {
			if *cgv.Name == *vm.Name {
				if cgv.AzureFile != nil {
					volumeConfig["share_name"] = *(*cgv.AzureFile).ShareName
					volumeConfig["storage_account_name"] = *(*cgv.AzureFile).StorageAccountName
					// skip storage_account_key, is always nil
				}
			}
		}

		// find corresponding volume in config
		// and use the data
		if containerVolumesConfig != nil {
			for _, cvr := range *containerVolumesConfig {
				cv := cvr.(map[string]interface{})
				rawName := cv["name"].(string)
				if *vm.Name == rawName {
					storageAccountKey := cv["storage_account_key"].(string)
					volumeConfig["storage_account_key"] = storageAccountKey
				}
			}
		}

		volumeConfigs = append(volumeConfigs, volumeConfig)
	}

	return volumeConfigs
}

func expandContainerGroupContainers(d *schema.ResourceData) (*[]containerinstance.Container, *[]containerinstance.Port, *[]containerinstance.Volume) {
	containersConfig := d.Get("container").([]interface{})
	containers := make([]containerinstance.Container, 0, len(containersConfig))
	containerGroupPorts := make([]containerinstance.Port, 0, len(containersConfig))
	containerGroupVolumes := make([]containerinstance.Volume, 0)

	for _, containerConfig := range containersConfig {
		data := containerConfig.(map[string]interface{})

		// required
		name := data["name"].(string)
		image := data["image"].(string)
		cpu := data["cpu"].(float64)
		memory := data["memory"].(float64)

		container := containerinstance.Container{
			Name: &name,
			ContainerProperties: &containerinstance.ContainerProperties{
				Image: &image,
				Resources: &containerinstance.ResourceRequirements{
					Requests: &containerinstance.ResourceRequests{
						MemoryInGB: &memory,
						CPU:        &cpu,
					},
				},
			},
		}

		if v, _ := data["port"]; v != 0 {
			port := int32(v.(int))

			// container port (port number)
			containerPort := containerinstance.ContainerPort{
				Port: &port,
			}
			container.Ports = &[]containerinstance.ContainerPort{containerPort}

			// container group port (port number + protocol)
			containerGroupPort := containerinstance.Port{
				Port: &port,
			}

			if v, ok := data["protocol"]; ok {
				protocol := v.(string)
				containerGroupPort.Protocol = containerinstance.ContainerGroupNetworkProtocol(strings.ToUpper(protocol))
			}

			containerGroupPorts = append(containerGroupPorts, containerGroupPort)
		}

		if v, ok := data["environment_variables"]; ok {
			container.EnvironmentVariables = expandContainerEnvironmentVariables(v)
		}

		if v, _ := data["command"]; v != "" {
			command := strings.Split(v.(string), " ")
			container.Command = &command
		}

		if v, ok := data["volume"]; ok {
			volumeMounts, containerGroupVolumesPartial := expandContainerVolumes(v)
			container.VolumeMounts = volumeMounts
			if containerGroupVolumesPartial != nil {
				containerGroupVolumes = append(containerGroupVolumes, *containerGroupVolumesPartial...)
			}
		}

		containers = append(containers, container)
	}

	return &containers, &containerGroupPorts, &containerGroupVolumes
}

func expandContainerEnvironmentVariables(input interface{}) *[]containerinstance.EnvironmentVariable {
	envVars := input.(map[string]interface{})
	output := make([]containerinstance.EnvironmentVariable, 0, len(envVars))

	for k, v := range envVars {
		ev := containerinstance.EnvironmentVariable{
			Name:  utils.String(k),
			Value: utils.String(v.(string)),
		}
		output = append(output, ev)
	}

	return &output
}

func expandContainerImageRegistryCredentials(d *schema.ResourceData) *[]containerinstance.ImageRegistryCredential {
	credsRaw := d.Get("image_registry_credential").([]interface{})
	if len(credsRaw) == 0 {
		return nil
	}

	output := make([]containerinstance.ImageRegistryCredential, 0, len(credsRaw))

	for _, c := range credsRaw {
		credConfig := c.(map[string]interface{})

		output = append(output, containerinstance.ImageRegistryCredential{
			Server:   utils.String(credConfig["server"].(string)),
			Password: utils.String(credConfig["password"].(string)),
			Username: utils.String(credConfig["username"].(string)),
		})
	}

	return &output
}

func flattenContainerImageRegistryCredentials(d *schema.ResourceData, credsPtr *[]containerinstance.ImageRegistryCredential) []interface{} {
	if credsPtr == nil {
		return nil
	}
	configsOld := d.Get("image_registry_credential").([]interface{})

	creds := *credsPtr
	output := make([]interface{}, 0, len(creds))
	for i, cred := range creds {
		credConfig := make(map[string]interface{})
		if cred.Server != nil {
			credConfig["server"] = *cred.Server
		}
		if cred.Username != nil {
			credConfig["username"] = *cred.Username
		}

		if len(configsOld) > i {
			data := configsOld[i].(map[string]interface{})
			oldServer := data["server"].(string)
			if cred.Server != nil && *cred.Server == oldServer {
				if v, ok := d.GetOk(fmt.Sprintf("image_registry_credential.%d.password", i)); ok {
					credConfig["password"] = v.(string)
				}
			}
		}

		output = append(output, credConfig)
	}
	return output
}

func expandContainerVolumes(input interface{}) (*[]containerinstance.VolumeMount, *[]containerinstance.Volume) {
	volumesRaw := input.([]interface{})

	if len(volumesRaw) == 0 {
		return nil, nil
	}

	volumeMounts := make([]containerinstance.VolumeMount, 0, len(volumesRaw))
	containerGroupVolumes := make([]containerinstance.Volume, 0, len(volumesRaw))

	for _, volumeRaw := range volumesRaw {
		volumeConfig := volumeRaw.(map[string]interface{})

		name := volumeConfig["name"].(string)
		mountPath := volumeConfig["mount_path"].(string)
		readOnly := volumeConfig["read_only"].(bool)
		shareName := volumeConfig["share_name"].(string)
		storageAccountName := volumeConfig["storage_account_name"].(string)
		storageAccountKey := volumeConfig["storage_account_key"].(string)

		vm := containerinstance.VolumeMount{
			Name:      &name,
			MountPath: &mountPath,
			ReadOnly:  &readOnly,
		}

		volumeMounts = append(volumeMounts, vm)

		cv := containerinstance.Volume{
			Name: &name,
			AzureFile: &containerinstance.AzureFileVolume{
				ShareName:          &shareName,
				ReadOnly:           &readOnly,
				StorageAccountName: &storageAccountName,
				StorageAccountKey:  &storageAccountKey,
			},
		}

		containerGroupVolumes = append(containerGroupVolumes, cv)
	}

	return &volumeMounts, &containerGroupVolumes
}
