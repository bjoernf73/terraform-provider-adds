Import-Module ActiveDirectory -ErrorAction Stop

$name = [string]$payload.name
$description = [string]$payload.description
if ([string]::IsNullOrEmpty($description)) {
    # New-ADObject/Set-ADObject reject a truly empty string attribute value.
    $descriptionValue = ' '
}
else {
    $descriptionValue = "$description "
}

$parm2 = ConvertTo-WmiFilterParm2 $payload.queries
$serverParams = Get-ServerParams

$existing = Get-WmiFilterByName $name
if ($null -eq $existing) {
    $guid = [System.Guid]::NewGuid().ToString().ToUpper()
    $cn = "{$guid}"
    $now = (Get-Date).ToUniversalTime().ToString('yyyyMMddHHmmss.ffffff-000')

    $attributes = @{
        'msWMI-Name'         = $name
        'msWMI-Parm1'        = $descriptionValue
        'msWMI-Parm2'        = $parm2
        'msWMI-Author'       = 'adds'
        'msWMI-ID'           = $cn
        instanceType         = 4
        showInAdvancedViewOnly = 'TRUE'
        'msWMI-ChangeDate'   = $now
        'msWMI-CreationDate' = $now
    }

    New-ADObject -Name $cn -Type 'msWMI-Som' -Path (Get-WmiFilterContainerDN) -OtherAttributes $attributes @serverParams -ErrorAction Stop | Out-Null
}
else {
    Set-ADObject -Identity $existing.DistinguishedName -Replace @{ 'msWMI-Parm1' = $descriptionValue; 'msWMI-Parm2' = $parm2 } @serverParams -ErrorAction Stop
}

$adObject = Get-WmiFilterByName $name
Get-WmiFilterResult $adObject | ConvertTo-Json -Compress -Depth 5
