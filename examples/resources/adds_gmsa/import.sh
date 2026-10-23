# Group managed service accounts are imported by objectGUID, which is stable across renames and moves.
terraform import adds_gmsa.websvc "b3f2b9a0-1234-4a85-a7a3-9bcbb6a41d02"
