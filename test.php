<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("abc");
$wpi->start();