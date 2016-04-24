<?php

ini_set('display_errors','1');
error_reporting(E_ALL);

require "wpi.php";

$wpi = new WPI("localhost:9999", "/var/www/wp");
$wpi->setToken("eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzY29wZSI6ImltcG9ydCIsImRhdGEiOiI0IiwiaWFwIjoxNDYxNTE4OTc1LCJleHAiOjE0NjE1MjI1NzV9.rHbfal-3nM5_M1zyPF8kzFN84BcGsVLL9aQI_DZuU9U");
$wpi->setVerbose(true);
$start = microtime(true);
$wpi->connect();
$output = $wpi->start();
$time_elapsed_secs = microtime(true) - $start;
echo $output . PHP_EOL;
die("Imported in ". $time_elapsed_secs . "\n");
