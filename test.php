<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzY29wZSI6ImltcG9ydCIsImRhdGEiOiI0IiwiaWFwIjoxNDYxNTcyNTA5LCJleHAiOjE0NjE1NzYxMDl9._5eusqy2m9qSn1_VEnqeqNyRyfx1_-swhYK1Aeb-uzE");
$wpi->setVerbose(true);
$start = microtime(true);
$wpi->connect();
$output = $wpi->start();
$time_elapsed_secs = microtime(true) - $start;
echo $output . PHP_EOL;
die("Imported in ". $time_elapsed_secs . "\n");
