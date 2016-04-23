<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("abc");
$wpi->setVerbose(true);
$start = microtime(true);
$wpi->connect();
$output = $wpi->start();
$time_elapsed_secs = microtime(true) - $start;
echo $output . PHP_EOL;
die("Imported in ". $time_elapsed_secs . "\n");
