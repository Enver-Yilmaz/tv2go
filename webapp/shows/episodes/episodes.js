angular.module('shows.episodes',[
  'shows.episodes.create',
  'shows.episodes.edit',
  'tv2go.models.shows',
  'tv2go.models.episodes',
])
.config(function($stateProvider){
  $stateProvider
    .state('tv2go.shows.episodes', {
      url: ":show",
      views: {
        'detail@tv2go': {
          templateUrl: 'shows/episodes/episodes.tmpl.html',
          controller: 'EpisodesListCtrl as episodesListCtrl',
        }
      },
      resolve: {
        show: function($stateParams, ShowsModel){
          return ShowsModel.getShowById($stateParams.show);
        },
        eps: function($stateParams, EpisodesModel) {
          return EpisodesModel.getEpisodes($stateParams.show);
        }
      },
    });
})
.controller('EpisodesListCtrl', function ($stateParams, show, eps) {
  var EpisodesListCtrl = this;
  
  EpisodesListCtrl.episodes = eps;
  EpisodesListCtrl.show = show;
})
;
