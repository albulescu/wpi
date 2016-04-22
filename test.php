<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("abc");
$start = microtime(true);
$wpi->connect();
$wpi->start();
$time_elapsed_secs = microtime(true) - $start;
die("Imported in ". $time_elapsed_secs . "\n");