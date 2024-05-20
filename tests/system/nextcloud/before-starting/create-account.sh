#!/bin/bash

# echo -ne fred | md5sum
export OC_PASS=570a90bfbf8c7eab5dc5d4e26832d5b1
php occ user:add --password-from-env --display-name="Fred" --group="users" fred
