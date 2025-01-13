#!/bin/bash

# echo -ne fred-password | md5sum
export OC_PASS=796ecbcc9e42a074e22b2c9aa03b79c6
php occ user:add --password-from-env --display-name="Fred" --group="users" fred
