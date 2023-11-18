import EmberRouter from '@ember/routing/router';

import config from 'crimeisdown/config/environment';

export default class Router extends EmberRouter {
  location = config.locationType;
  rootURL = config.rootURL;
}

Router.map(function () {
  this.route('not-found', {
    path: '/*path',
  });
  this.route('search-transcripts', {
    path: '/transcripts/search',
  });
});
